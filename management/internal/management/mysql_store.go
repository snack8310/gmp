package management

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-sql-driver/mysql"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type managementMigration struct {
	id   string
	file string
}

var managementMigrations = []managementMigration{
	{id: "001_management", file: "migrations/001_management.sql"},
	{id: "002_execution_receipts", file: "migrations/002_execution_receipts.sql"},
}

type MySQLStore struct {
	db     *sql.DB
	logger *slog.Logger
}

type sqlQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func OpenMySQLStore(ctx context.Context, config MySQLConfig, logger *slog.Logger) (*MySQLStore, error) {
	if strings.TrimSpace(config.DSN) == "" || config.MaxOpenConns <= 0 || config.MaxIdleConns < 0 ||
		config.MaxIdleConns > config.MaxOpenConns || config.ConnMaxLifetime <= 0 {
		return nil, errors.New("management: invalid MySQL configuration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	parsed, err := mysql.ParseDSN(config.DSN)
	if err != nil || parsed.DBName == "" {
		return nil, errors.New("management: MYSQL_DSN must be valid and include a database name")
	}
	parsed.ParseTime = true
	db, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		logMySQLFailure(logger, "open_connection", err)
		return nil, fmt.Errorf("management: opening MySQL driver: %w", err)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		logMySQLFailure(logger, "check_connection", err)
		return nil, errors.New("management: MySQL connection check failed")
	}
	store := &MySQLStore{db: db, logger: logger}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	logger.Info("mysql_store_ready", "result", "success")
	return store, nil
}

func (s *MySQLStore) Close() error {
	err := s.db.Close()
	if err != nil {
		s.logFailure("close_connection_pool", err)
		return err
	}
	s.logger.Info("mysql_store_operation", "operation", "close_connection_pool", "result", "success")
	return nil
}

// ExecutionLog returns a persistent campaign log backed by the store's pool.
func (s *MySQLStore) ExecutionLog() *MySQLExecutionLog {
	return newMySQLExecutionLog(s.db, s.logger)
}

func (s *MySQLStore) migrate(ctx context.Context) error {
	first, err := fs.ReadFile(migrationFiles, managementMigrations[0].file)
	if err != nil {
		return fmt.Errorf("management: reading embedded MySQL migration: %w", err)
	}
	firstStatements := splitSQLStatements(string(first))
	if len(firstStatements) == 0 {
		return errors.New("management: embedded MySQL migration is empty")
	}
	if _, err := s.db.ExecContext(ctx, firstStatements[0]); err != nil {
		s.logFailure("migration_table", err)
		return errors.New("management: creating MySQL migration ledger failed")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT migration_id FROM gmp_management_schema_migrations")
	if err != nil {
		s.logFailure("migration_read", err)
		return errors.New("management: reading MySQL migration ledger failed")
	}
	applied := make(map[string]bool, len(managementMigrations))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			s.logFailure("migration_read", err)
			return errors.New("management: reading MySQL migration ledger failed")
		}
		known := false
		for _, migration := range managementMigrations {
			if migration.id == id {
				known = true
				applied[id] = true
				break
			}
		}
		if !known {
			_ = rows.Close()
			return fmt.Errorf("management: unsupported MySQL schema migration %q", id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		s.logFailure("migration_read", err)
		return errors.New("management: reading MySQL migration ledger failed")
	}
	if err := rows.Close(); err != nil {
		return errors.New("management: closing MySQL migration result failed")
	}
	for _, migration := range managementMigrations {
		if applied[migration.id] {
			continue
		}
		raw, err := fs.ReadFile(migrationFiles, migration.file)
		if err != nil {
			return fmt.Errorf("management: reading embedded MySQL migration %s: %w", migration.id, err)
		}
		statements := splitSQLStatements(string(raw))
		if len(statements) == 0 {
			return fmt.Errorf("management: embedded MySQL migration %s is empty", migration.id)
		}
		start := 0
		if migration.id == managementMigrations[0].id {
			start = 1
		}
		for _, statement := range statements[start:] {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				s.logFailure("migration_apply", err)
				return errors.New("management: applying MySQL schema migration failed")
			}
		}
		if _, err := s.db.ExecContext(ctx, "INSERT IGNORE INTO gmp_management_schema_migrations (migration_id) VALUES (?)", migration.id); err != nil {
			s.logFailure("migration_record", err)
			return errors.New("management: recording MySQL schema migration failed")
		}
		s.logger.Info("mysql_schema_migration", "migration_id", migration.id, "result", "applied")
	}
	return s.verifySchema(ctx)
}

func (s *MySQLStore) verifySchema(ctx context.Context) error {
	if err := verifyManagementSchema(ctx, s.db); err != nil {
		s.logFailure("schema_check", err)
		return errors.New("management: required MySQL schema is unavailable")
	}
	return nil
}

func splitSQLStatements(script string) []string {
	var statements []string
	for _, raw := range strings.Split(script, ";") {
		statement := strings.TrimSpace(raw)
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}

func (s *MySQLStore) logFailure(operation string, err error) {
	logMySQLFailure(s.logger, operation, err)
}

func logMySQLFailure(logger *slog.Logger, operation string, err error) {
	logger.Error("mysql_store_operation", "operation", operation, "result", "failed", "error_type", fmt.Sprintf("%T", err))
}

func storageProblem() *Problem {
	return problem(http.StatusInternalServerError, "storage_unavailable", "持久化服务暂不可用")
}

func readMySQL[T any](s *MySQLStore, operation string, apply func(*Store) (T, *Problem)) (T, *Problem) {
	var zero T
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	defer tx.Rollback()
	state, err := loadStoreState(ctx, tx)
	if err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	value, p := apply(state)
	if p != nil {
		return value, p
	}
	if err := tx.Commit(); err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	s.logger.Info("mysql_store_operation", "operation", operation, "result", "success")
	return value, nil
}

func writeMySQL[T any](s *MySQLStore, operation string, apply func(*Store) (T, *Problem)) (T, *Problem) {
	var zero T
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	defer tx.Rollback()
	var lockID int
	if err := tx.QueryRowContext(ctx, "SELECT lock_id FROM gmp_management_write_lock WHERE lock_id = 1 FOR UPDATE").Scan(&lockID); err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	state, err := loadStoreState(ctx, tx)
	if err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	value, p := apply(state)
	if p != nil {
		return value, p
	}
	if err := saveStoreState(ctx, tx, state); err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	if err := tx.Commit(); err != nil {
		s.logFailure(operation, err)
		return zero, storageProblem()
	}
	s.logger.Info("mysql_store_operation", "operation", operation, "result", "success")
	return value, nil
}

func (s *MySQLStore) Actions() []Action {
	return newStore(false).Actions()
}

func (s *MySQLStore) AudienceReferences() ([]AudienceReference, *Problem) {
	return readMySQL(s, "list_audience_references", (*Store).AudienceReferences)
}

func (s *MySQLStore) Campaigns() ([]Campaign, *Problem) {
	return readMySQL(s, "list_campaigns", (*Store).Campaigns)
}

func (s *MySQLStore) Campaign(id string) (Campaign, *Problem) {
	return readMySQL(s, "get_campaign", func(state *Store) (Campaign, *Problem) { return state.Campaign(id) })
}

func (s *MySQLStore) CreateCampaign(name string) (Campaign, *Problem) {
	return writeMySQL(s, "create_campaign", func(state *Store) (Campaign, *Problem) { return state.CreateCampaign(name) })
}

func (s *MySQLStore) UpdateCampaign(id string, next Campaign) (Campaign, *Problem) {
	return writeMySQL(s, "update_campaign", func(state *Store) (Campaign, *Problem) { return state.UpdateCampaign(id, next) })
}

func (s *MySQLStore) UpdateFlow(campaignID string, expectedEntries []EntryRoute, expectedEdges []Edge, flow Flow) (Campaign, *Problem) {
	return writeMySQL(s, "update_flow", func(state *Store) (Campaign, *Problem) {
		return state.UpdateFlow(campaignID, expectedEntries, expectedEdges, flow)
	})
}

func (s *MySQLStore) UpdateAudienceSelection(campaignID string, expected, nextSelection *AudienceSelection) (Campaign, *Problem) {
	return writeMySQL(s, "update_audience", func(state *Store) (Campaign, *Problem) {
		return state.UpdateAudienceSelection(campaignID, expected, nextSelection)
	})
}

func (s *MySQLStore) DeleteCampaign(id string) *Problem {
	_, p := writeMySQL(s, "delete_campaign", func(state *Store) (struct{}, *Problem) {
		return struct{}{}, state.DeleteCampaign(id)
	})
	return p
}

func (s *MySQLStore) CreateTreatment(campaignID string, treatment Treatment) (Treatment, *Problem) {
	return writeMySQL(s, "create_treatment", func(state *Store) (Treatment, *Problem) { return state.CreateTreatment(campaignID, treatment) })
}

func (s *MySQLStore) UpdateTreatment(campaignID, treatmentID string, treatment Treatment) (Treatment, *Problem) {
	return writeMySQL(s, "update_treatment", func(state *Store) (Treatment, *Problem) {
		return state.UpdateTreatment(campaignID, treatmentID, treatment)
	})
}

func (s *MySQLStore) DeleteTreatment(campaignID, treatmentID string) *Problem {
	_, p := writeMySQL(s, "delete_treatment", func(state *Store) (struct{}, *Problem) {
		return struct{}{}, state.DeleteTreatment(campaignID, treatmentID)
	})
	return p
}

func (s *MySQLStore) UpsertGroupAudienceReference(groupID, name string) (AudienceReference, *Problem) {
	return writeMySQL(s, "upsert_audience_reference", func(state *Store) (AudienceReference, *Problem) {
		return state.UpsertGroupAudienceReference(groupID, name)
	})
}

func (s *MySQLStore) DeleteGroupAudienceReference(groupID string) *Problem {
	_, p := writeMySQL(s, "delete_audience_reference", func(state *Store) (struct{}, *Problem) {
		return struct{}{}, state.DeleteGroupAudienceReference(groupID)
	})
	return p
}

func (s *MySQLStore) Usages(ref AudienceRef) ([]Usage, *Problem) {
	return readMySQL(s, "get_audience_usages", func(state *Store) ([]Usage, *Problem) { return state.Usages(ref) })
}
