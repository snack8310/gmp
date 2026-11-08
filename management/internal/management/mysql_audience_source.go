package management

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var mysqlIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var demoAudienceSchemaContract = mysqlSchemaTable{
	name: "gmp_demo_user_pool",
	columns: []mysqlSchemaColumn{
		{name: "member_id", sqlType: "bigint unsigned", autoIncrement: true},
		{name: "uid", sqlType: "varchar(128)"},
		{name: "region", sqlType: "varchar(64)"},
		{name: "lifecycle", sqlType: "varchar(32)"},
		{name: "inactive_days", sqlType: "int"},
		{name: "tier", sqlType: "varchar(32)"},
	},
	uniqueKeys: [][]string{{"member_id"}, {"uid"}},
}

// MySQLAudienceSourceConfig describes an explicit projection from a source
// table or view. UID uniqueness and cursor uniqueness/stability are asserted
// by the caller because MySQL does not expose unique-index metadata for views.
type MySQLAudienceSourceConfig struct {
	Table        string
	UIDColumn    string
	UIDUnique    bool
	CursorColumn string
	CursorUnique bool
	CursorStable bool
	Attributes   map[string]string
	PageSize     int
}

// AudienceMember is a transient source projection. It is not persisted by
// this adapter.
type AudienceMember struct {
	UID        audience.UID
	Attributes map[string]string
}

// AudienceMemberPage contains one bounded read and an opaque continuation
// cursor. The caller must keep the cursor out of logs and persistent snapshots.
type AudienceMemberPage struct {
	Members    []AudienceMember
	NextCursor *string
}

// MySQLAudienceSource exposes a read-only, explicitly mapped source query.
// It has no cache and no write path.
type MySQLAudienceSource struct {
	db             *sql.DB
	qualifiedTable string
	database       string
	table          string
	cfg            MySQLAudienceSourceConfig
	attributeNames []string
	logger         *slog.Logger
}

// DemoAudienceSourceConfig returns the repository-owned synthetic user pool
// projection. The page size is explicit so callers cannot inherit an invented
// production default.
func DemoAudienceSourceConfig(pageSize int) (MySQLAudienceSourceConfig, error) {
	maxInt := int(^uint(0) >> 1)
	if pageSize <= 0 || pageSize == maxInt {
		return MySQLAudienceSourceConfig{}, errors.New("management: demo audience page size must be a positive integer below the platform limit")
	}
	return MySQLAudienceSourceConfig{
		Table:        "gmp_demo_user_pool",
		UIDColumn:    "uid",
		UIDUnique:    true,
		CursorColumn: "member_id",
		CursorUnique: true,
		CursorStable: true,
		Attributes: map[string]string{
			"region":        "region",
			"lifecycle":     "lifecycle",
			"inactive_days": "inactive_days",
			"tier":          "tier",
		},
		PageSize: pageSize,
	}, nil
}

// NewDemoAudienceSource reads the constructed demo pool through this store's
// existing connection pool. The source adapter only issues SELECT statements.
func (s *MySQLStore) NewDemoAudienceSource(pageSize int, logger *slog.Logger) (*MySQLAudienceSource, error) {
	cfg, err := DemoAudienceSourceConfig(pageSize)
	if err != nil {
		return nil, err
	}
	if err := verifyMySQLTable(context.Background(), s.db, demoAudienceSchemaContract); err != nil {
		s.logger.Error("mysql_audience_source_schema", "operation", "validate_demo_pool", "result", "failed", "error_type", fmt.Sprintf("%T", err))
		return nil, errors.New("management: demo audience table does not match its declared schema")
	}
	s.logger.Info("mysql_audience_source_schema", "operation", "validate_demo_pool", "result", "success")
	if logger == nil {
		logger = s.logger
	}
	return NewMySQLAudienceSource(s.db, cfg, logger)
}

// NewMySQLAudienceSource validates the configured projection and confirms
// that the relation and mapped columns exist. It does not read member rows.
func NewMySQLAudienceSource(db *sql.DB, cfg MySQLAudienceSourceConfig, logger *slog.Logger) (*MySQLAudienceSource, error) {
	qualifiedTable, database, table, err := parseMySQLRelation(cfg.Table)
	if err != nil {
		return nil, err
	}
	if !mysqlIdentifier.MatchString(cfg.UIDColumn) || !mysqlIdentifier.MatchString(cfg.CursorColumn) {
		return nil, errors.New("management: audience UID and cursor columns must be simple SQL identifiers")
	}
	if cfg.UIDColumn == cfg.CursorColumn {
		return nil, errors.New("management: audience UID and cursor columns must be distinct")
	}
	if !cfg.UIDUnique {
		return nil, errors.New("management: audience UID column must be explicitly declared unique")
	}
	if !cfg.CursorUnique || !cfg.CursorStable {
		return nil, errors.New("management: audience cursor must be explicitly declared unique and stable")
	}
	if cfg.Attributes == nil {
		return nil, errors.New("management: audience attribute mapping must be explicit")
	}
	for name, column := range cfg.Attributes {
		if strings.TrimSpace(name) == "" || !mysqlIdentifier.MatchString(column) {
			return nil, errors.New("management: audience attribute names must be non-empty and mapped columns must be simple SQL identifiers")
		}
	}
	maxInt := int(^uint(0) >> 1)
	if cfg.PageSize <= 0 || cfg.PageSize == maxInt {
		return nil, errors.New("management: audience page size must be a positive integer below the platform limit")
	}
	if db == nil {
		return nil, errors.New("management: audience source database handle is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	attributes := make(map[string]string, len(cfg.Attributes))
	for name, column := range cfg.Attributes {
		attributes[name] = column
	}
	cfg.Attributes = attributes

	source := &MySQLAudienceSource{
		db:             db,
		qualifiedTable: qualifiedTable,
		database:       database,
		table:          table,
		cfg:            cfg,
		logger:         logger,
	}
	for name := range cfg.Attributes {
		source.attributeNames = append(source.attributeNames, name)
	}
	sort.Strings(source.attributeNames)
	if err := source.validateColumns(context.Background()); err != nil {
		return nil, err
	}
	return source, nil
}

func parseMySQLRelation(relation string) (qualified, database, table string, err error) {
	parts := strings.Split(relation, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return "", "", "", errors.New("management: audience source table must be table or database.table")
	}
	for _, part := range parts {
		if !mysqlIdentifier.MatchString(part) {
			return "", "", "", errors.New("management: audience source table must use simple SQL identifiers")
		}
	}
	if len(parts) == 1 {
		return quoteMySQLIdentifier(parts[0]), "", parts[0], nil
	}
	return quoteMySQLIdentifier(parts[0]) + "." + quoteMySQLIdentifier(parts[1]), parts[0], parts[1], nil
}

func quoteMySQLIdentifier(value string) string { return "`" + value + "`" }

func (s *MySQLAudienceSource) validateColumns(ctx context.Context) error {
	started := time.Now()
	database := s.database
	if database == "" {
		if err := s.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
			s.logFailure("validate_source_schema", started, err)
			return errors.New("management: audience source schema lookup failed")
		}
		if database == "" {
			s.logger.Error("mysql_audience_source_schema", "operation", "validate_source_schema", "result", "failed", "reason", "database_not_selected", "duration_ms", time.Since(started).Milliseconds())
			return errors.New("management: audience source database must be selected")
		}
		s.database = database
	}

	rows, err := s.db.QueryContext(ctx, "SELECT COLUMN_NAME, IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?", database, s.table)
	if err != nil {
		s.logFailure("validate_source_schema", started, err)
		return errors.New("management: audience source schema validation failed")
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var name, nullable string
		if err := rows.Scan(&name, &nullable); err != nil {
			_ = rows.Close()
			s.logFailure("validate_source_schema", started, err)
			return errors.New("management: audience source schema validation failed")
		}
		columns[name] = nullable == "NO"
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		s.logFailure("validate_source_schema", started, err)
		return errors.New("management: audience source schema validation failed")
	}
	if err := rows.Close(); err != nil {
		s.logFailure("validate_source_schema", started, err)
		return errors.New("management: audience source schema validation failed")
	}
	if len(columns) == 0 {
		s.logger.Error("mysql_audience_source_schema", "operation", "validate_source_schema", "result", "failed", "reason", "relation_not_found", "duration_ms", time.Since(started).Milliseconds())
		return errors.New("management: configured audience source relation was not found")
	}
	if _, ok := columns[s.cfg.UIDColumn]; !ok || !columns[s.cfg.UIDColumn] {
		s.logger.Error("mysql_audience_source_schema", "operation", "validate_source_schema", "result", "failed", "reason", "uid_column_missing_or_nullable", "duration_ms", time.Since(started).Milliseconds())
		return errors.New("management: configured audience UID column must exist and be non-null")
	}
	if _, ok := columns[s.cfg.CursorColumn]; !ok || !columns[s.cfg.CursorColumn] {
		s.logger.Error("mysql_audience_source_schema", "operation", "validate_source_schema", "result", "failed", "reason", "cursor_column_missing_or_nullable", "duration_ms", time.Since(started).Milliseconds())
		return errors.New("management: configured audience cursor column must exist and be non-null")
	}
	for _, name := range s.attributeNames {
		if _, ok := columns[s.cfg.Attributes[name]]; !ok {
			s.logger.Error("mysql_audience_source_schema", "operation", "validate_source_schema", "result", "failed", "reason", "attribute_column_missing", "duration_ms", time.Since(started).Milliseconds())
			return errors.New("management: configured audience attribute column was not found")
		}
	}
	s.logger.Info("mysql_audience_source_schema", "operation", "validate_source_schema", "result", "success", "duration_ms", time.Since(started).Milliseconds())
	return nil
}

// ReadPage reads a bounded page using keyset pagination. It projects only the
// configured UID and attribute columns and never logs cursor or member values.
func (s *MySQLAudienceSource) ReadPage(ctx context.Context, cursor *string) (AudienceMemberPage, error) {
	if ctx == nil {
		return AudienceMemberPage{}, errors.New("management: audience source context is required")
	}
	started := time.Now()
	columns := []string{s.cfg.CursorColumn, s.cfg.UIDColumn}
	for _, name := range s.attributeNames {
		columns = append(columns, s.cfg.Attributes[name])
	}
	quotedColumns := make([]string, len(columns))
	for index, column := range columns {
		quotedColumns[index] = quoteMySQLIdentifier(column)
	}
	query := "SELECT " + strings.Join(quotedColumns, ", ") + " FROM " + s.qualifiedTable
	args := make([]any, 0, 2)
	if cursor != nil {
		query += " WHERE " + quoteMySQLIdentifier(s.cfg.CursorColumn) + " > ?"
		args = append(args, *cursor)
	}
	query += " ORDER BY " + quoteMySQLIdentifier(s.cfg.CursorColumn) + " LIMIT ?"
	args = append(args, s.cfg.PageSize+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.logFailure("read_member_page", started, err)
		return AudienceMemberPage{}, errors.New("management: audience source page read failed")
	}
	type scannedMember struct {
		cursor string
		member AudienceMember
	}
	items := make([]scannedMember, 0)
	for rows.Next() {
		var cursorValue, uid sql.NullString
		attributes := make([]sql.NullString, len(s.attributeNames))
		destinations := make([]any, 0, len(columns))
		destinations = append(destinations, &cursorValue, &uid)
		for index := range attributes {
			destinations = append(destinations, &attributes[index])
		}
		if err := rows.Scan(destinations...); err != nil {
			_ = rows.Close()
			s.logFailure("read_member_page", started, err)
			return AudienceMemberPage{}, errors.New("management: audience source page read failed")
		}
		if !cursorValue.Valid || !uid.Valid || uid.String == "" {
			_ = rows.Close()
			s.logger.Error("mysql_audience_source_read", "operation", "read_member_page", "result", "failed", "reason", "null_cursor_or_uid", "duration_ms", time.Since(started).Milliseconds())
			return AudienceMemberPage{}, errors.New("management: audience source returned a null cursor or empty UID")
		}
		member := AudienceMember{UID: audience.UID(uid.String), Attributes: make(map[string]string)}
		for index, value := range attributes {
			if value.Valid {
				member.Attributes[s.attributeNames[index]] = value.String
			}
		}
		items = append(items, scannedMember{cursor: cursorValue.String, member: member})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		s.logFailure("read_member_page", started, err)
		return AudienceMemberPage{}, errors.New("management: audience source page read failed")
	}
	if err := rows.Close(); err != nil {
		s.logFailure("read_member_page", started, err)
		return AudienceMemberPage{}, errors.New("management: audience source page read failed")
	}

	hasMore := len(items) > s.cfg.PageSize
	if hasMore {
		items = items[:s.cfg.PageSize]
	}
	page := AudienceMemberPage{Members: make([]AudienceMember, len(items))}
	for index, item := range items {
		page.Members[index] = item.member
	}
	if hasMore {
		last := items[len(items)-1].cursor
		page.NextCursor = &last
	}
	s.logger.Info("mysql_audience_source_read", "operation", "read_member_page", "result", "success", "row_count", len(page.Members), "has_more", hasMore, "duration_ms", time.Since(started).Milliseconds())
	return page, nil
}

func (s *MySQLAudienceSource) logFailure(operation string, started time.Time, err error) {
	s.logger.Error("mysql_audience_source_read", "operation", operation, "result", "failed", "error_type", fmt.Sprintf("%T", err), "duration_ms", time.Since(started).Milliseconds())
}
