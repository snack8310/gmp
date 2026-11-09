package management

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

var (
	ErrExecutionKeyCollision    = errors.New("management: execution key hash collision")
	ErrExecutionReceiptRequired = errors.New("management: accepting a receipt requires a receipted execution")
)

// MySQLExecutionLog stores campaign executions and their first accepted receipt.
type MySQLExecutionLog struct {
	db     *sql.DB
	logger *slog.Logger
}

func newMySQLExecutionLog(db *sql.DB, logger *slog.Logger) *MySQLExecutionLog {
	return &MySQLExecutionLog{db: db, logger: logger}
}

func (l *MySQLExecutionLog) Append(execution campaign.Execution) error {
	return l.AppendContext(context.Background(), execution)
}

func (l *MySQLExecutionLog) AppendContext(ctx context.Context, execution campaign.Execution) error {
	started := time.Now()
	hash := executionHash(execution.Key)
	placements, err := json.Marshal(execution.Placements)
	if err != nil {
		return l.failure("append_execution", started, err)
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return l.failure("append_execution", started, err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		"INSERT INTO gmp_management_execution_records (execution_hash, execution_key, campaign_id, treatment_id, uid, action_id, placements, state, attempts, external_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		hash[:], string(execution.Key), string(execution.Campaign), string(execution.Treatment), string(execution.UID), string(execution.Action), placements, string(execution.State), execution.Attempts, execution.ExternalRef,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
			return l.failure("append_execution", started, err)
		}
		var existingKey string
		if readErr := tx.QueryRowContext(ctx,
			"SELECT execution_key FROM gmp_management_execution_records WHERE execution_hash = ?", hash[:],
		).Scan(&existingKey); readErr != nil {
			return l.failure("append_execution", started, readErr)
		}
		if existingKey != string(execution.Key) {
			return l.failure("append_execution", started, ErrExecutionKeyCollision)
		}
		if err := tx.Commit(); err != nil {
			return l.failure("append_execution", started, err)
		}
		l.success("append_execution", started)
		return nil
	}
	if execution.HasReceipt() {
		if err := insertExecutionReceipt(ctx, tx, hash[:], execution); err != nil {
			return l.failure("append_execution", started, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return l.failure("append_execution", started, err)
	}
	l.success("append_execution", started)
	return nil
}

func (l *MySQLExecutionLog) Update(execution campaign.Execution) error {
	return l.UpdateContext(context.Background(), execution)
}

func (l *MySQLExecutionLog) UpdateContext(ctx context.Context, execution campaign.Execution) error {
	started := time.Now()
	hash := executionHash(execution.Key)
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return l.failure("update_execution", started, err)
	}
	defer tx.Rollback()

	previous, found, err := loadExecutionForUpdate(ctx, tx, hash[:])
	if err != nil {
		return l.failure("update_execution", started, err)
	}
	if !found {
		return l.failure("update_execution", started, fmt.Errorf("%w: %q", campaign.ErrNoSuchExecution, execution.Key))
	}
	if previous.Key != execution.Key {
		return l.failure("update_execution", started, ErrExecutionKeyCollision)
	}
	previousReceipt, hasReceipt, err := loadExecutionReceipt(ctx, tx, hash[:], previous)
	if err != nil {
		return l.failure("update_execution", started, err)
	}
	if hasReceipt && !execution.MatchesReceipt(previousReceipt) {
		return l.conflict("update_execution", started)
	}

	placements, err := json.Marshal(execution.Placements)
	if err != nil {
		return l.failure("update_execution", started, err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE gmp_management_execution_records SET campaign_id = ?, treatment_id = ?, uid = ?, action_id = ?, placements = ?, state = ?, attempts = ?, external_ref = ? WHERE execution_hash = ?",
		string(execution.Campaign), string(execution.Treatment), string(execution.UID), string(execution.Action), placements, string(execution.State), execution.Attempts, execution.ExternalRef, hash[:],
	); err != nil {
		return l.failure("update_execution", started, err)
	}
	if !hasReceipt && execution.HasReceipt() {
		if err := insertExecutionReceipt(ctx, tx, hash[:], execution); err != nil {
			return l.failure("update_execution", started, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return l.failure("update_execution", started, err)
	}
	l.success("update_execution", started)
	return nil
}

func (l *MySQLExecutionLog) AcceptReceipt(execution campaign.Execution) (bool, error) {
	return l.AcceptReceiptContext(context.Background(), execution)
}

func (l *MySQLExecutionLog) AcceptReceiptContext(ctx context.Context, execution campaign.Execution) (bool, error) {
	started := time.Now()
	if !execution.HasReceipt() {
		return false, l.failure("accept_receipt", started, ErrExecutionReceiptRequired)
	}
	hash := executionHash(execution.Key)
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	defer tx.Rollback()

	previous, found, err := loadExecutionForUpdate(ctx, tx, hash[:])
	if err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	if !found {
		return false, l.failure("accept_receipt", started, fmt.Errorf("%w: %q", campaign.ErrNoSuchExecution, execution.Key))
	}
	if previous.Key != execution.Key {
		return false, l.failure("accept_receipt", started, ErrExecutionKeyCollision)
	}
	previousReceipt, hasReceipt, err := loadExecutionReceipt(ctx, tx, hash[:], previous)
	if err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	if hasReceipt {
		if !execution.MatchesReceipt(previousReceipt) {
			return false, l.conflict("accept_receipt", started)
		}
		if err := tx.Commit(); err != nil {
			return false, l.failure("accept_receipt", started, err)
		}
		l.replayed("accept_receipt", started)
		return false, nil
	}

	placements, err := json.Marshal(execution.Placements)
	if err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE gmp_management_execution_records SET campaign_id = ?, treatment_id = ?, uid = ?, action_id = ?, placements = ?, state = ?, attempts = ?, external_ref = ? WHERE execution_hash = ?",
		string(execution.Campaign), string(execution.Treatment), string(execution.UID), string(execution.Action), placements, string(execution.State), execution.Attempts, execution.ExternalRef, hash[:],
	); err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	if err := insertExecutionReceipt(ctx, tx, hash[:], execution); err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	if err := tx.Commit(); err != nil {
		return false, l.failure("accept_receipt", started, err)
	}
	l.success("accept_receipt", started)
	return true, nil
}

func (l *MySQLExecutionLog) Lookup(key campaign.IdempotencyKey) (campaign.Execution, bool, error) {
	return l.LookupContext(context.Background(), key)
}

func (l *MySQLExecutionLog) LookupContext(ctx context.Context, key campaign.IdempotencyKey) (campaign.Execution, bool, error) {
	started := time.Now()
	hash := executionHash(key)
	execution, found, err := readExecution(ctx, l.db, hash[:])
	if err != nil {
		return campaign.Execution{}, false, l.failure("lookup_execution", started, err)
	}
	if found && execution.Key != key {
		return campaign.Execution{}, false, l.failure("lookup_execution", started, ErrExecutionKeyCollision)
	}
	l.success("lookup_execution", started)
	return execution, found, nil
}

func (l *MySQLExecutionLog) All() ([]campaign.Execution, error) {
	return l.AllContext(context.Background())
}

func (l *MySQLExecutionLog) AllContext(ctx context.Context) ([]campaign.Execution, error) {
	started := time.Now()
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, l.failure("list_executions", started, err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx,
		"SELECT e.execution_hash FROM gmp_management_execution_records AS e ORDER BY BINARY e.execution_key",
	)
	if err != nil {
		return nil, l.failure("list_executions", started, err)
	}
	type rowKey struct{ hash []byte }
	keys := make([]rowKey, 0)
	for rows.Next() {
		var item rowKey
		if err := rows.Scan(&item.hash); err != nil {
			_ = rows.Close()
			return nil, l.failure("list_executions", started, err)
		}
		keys = append(keys, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, l.failure("list_executions", started, err)
	}
	if err := rows.Close(); err != nil {
		return nil, l.failure("list_executions", started, err)
	}
	out := make([]campaign.Execution, 0, len(keys))
	for _, key := range keys {
		execution, found, err := readExecution(ctx, tx, key.hash)
		if err != nil {
			return nil, l.failure("list_executions", started, err)
		}
		if !found {
			return nil, l.failure("list_executions", started, errors.New("management: execution disappeared during listing"))
		}
		out = append(out, execution)
	}
	if err := tx.Commit(); err != nil {
		return nil, l.failure("list_executions", started, err)
	}
	l.success("list_executions", started)
	return out, nil
}

func loadExecutionForUpdate(ctx context.Context, tx *sql.Tx, hash []byte) (campaign.Execution, bool, error) {
	var (
		key, campaignID, treatmentID, uid, actionID, state, externalRef string
		placementsJSON                                                  []byte
		attempts                                                        int
	)
	err := tx.QueryRowContext(ctx,
		"SELECT execution_key, campaign_id, treatment_id, uid, action_id, placements, state, attempts, external_ref FROM gmp_management_execution_records WHERE execution_hash = ? FOR UPDATE",
		hash,
	).Scan(&key, &campaignID, &treatmentID, &uid, &actionID, &placementsJSON, &state, &attempts, &externalRef)
	if errors.Is(err, sql.ErrNoRows) {
		return campaign.Execution{}, false, nil
	}
	if err != nil {
		return campaign.Execution{}, false, err
	}
	execution := campaign.Execution{
		Key: campaign.IdempotencyKey(key), Campaign: campaign.CampaignID(campaignID),
		Treatment: campaign.TreatmentID(treatmentID), UID: audience.UID(uid), Action: campaign.ActionID(actionID),
		State: campaign.State(state), Attempts: attempts, ExternalRef: externalRef,
	}
	if err := json.Unmarshal(placementsJSON, &execution.Placements); err != nil {
		return campaign.Execution{}, false, err
	}
	return execution, true, nil
}

func readExecution(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, hash []byte) (campaign.Execution, bool, error) {
	var (
		key, campaignID, treatmentID, uid, actionID, state, externalRef string
		placementsJSON, receiptHash, receiptAttributesJSON              []byte
		attempts                                                        int
		arrived                                                         sql.NullBool
		outcome, receiptExternalRef                                     sql.NullString
	)
	err := db.QueryRowContext(ctx,
		"SELECT e.execution_key, e.campaign_id, e.treatment_id, e.uid, e.action_id, e.placements, e.state, e.attempts, e.external_ref, r.execution_hash, r.arrived, r.outcome, r.external_ref, r.attributes FROM gmp_management_execution_records AS e LEFT JOIN gmp_management_execution_receipts AS r ON r.execution_hash = e.execution_hash WHERE e.execution_hash = ?",
		hash,
	).Scan(&key, &campaignID, &treatmentID, &uid, &actionID, &placementsJSON, &state, &attempts, &externalRef, &receiptHash, &arrived, &outcome, &receiptExternalRef, &receiptAttributesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return campaign.Execution{}, false, nil
	}
	if err != nil {
		return campaign.Execution{}, false, err
	}
	execution := campaign.Execution{
		Key: campaign.IdempotencyKey(key), Campaign: campaign.CampaignID(campaignID),
		Treatment: campaign.TreatmentID(treatmentID), UID: audience.UID(uid), Action: campaign.ActionID(actionID),
		State: campaign.State(state), Attempts: attempts, ExternalRef: externalRef,
	}
	if err := json.Unmarshal(placementsJSON, &execution.Placements); err != nil {
		return campaign.Execution{}, false, err
	}
	if receiptHash != nil {
		if !arrived.Valid || !outcome.Valid || !receiptExternalRef.Valid {
			return campaign.Execution{}, false, errors.New("management: persisted receipt is incomplete")
		}
		execution.Outcome = campaign.Outcome(outcome.String)
		execution.ReceiptExternalRef = receiptExternalRef.String
		execution.ReceiptAttributes = make(map[string]string)
		if err := json.Unmarshal(receiptAttributesJSON, &execution.ReceiptAttributes); err != nil {
			return campaign.Execution{}, false, err
		}
		if arrived.Bool {
			execution.State = campaign.StateArrived
		} else {
			execution.State = campaign.StateNotArrived
		}
		if state != string(execution.State) {
			return campaign.Execution{}, false, errors.New("management: persisted execution and receipt state disagree")
		}
	}
	return execution, true, nil
}

func loadExecutionReceipt(ctx context.Context, tx *sql.Tx, hash []byte, execution campaign.Execution) (campaign.Receipt, bool, error) {
	var (
		arrived              bool
		outcome, externalRef string
		attributesJSON       []byte
	)
	err := tx.QueryRowContext(ctx,
		"SELECT arrived, outcome, external_ref, attributes FROM gmp_management_execution_receipts WHERE execution_hash = ?",
		hash,
	).Scan(&arrived, &outcome, &externalRef, &attributesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return campaign.Receipt{}, false, nil
	}
	if err != nil {
		return campaign.Receipt{}, false, err
	}
	var attributes map[string]string
	if err := json.Unmarshal(attributesJSON, &attributes); err != nil {
		return campaign.Receipt{}, false, err
	}
	return campaign.Receipt{
		Key: execution.Key, Arrived: arrived, Outcome: campaign.Outcome(outcome),
		ExternalRef: externalRef, Attributes: attributes,
	}, true, nil
}

func insertExecutionReceipt(ctx context.Context, tx *sql.Tx, hash []byte, execution campaign.Execution) error {
	attributes, err := json.Marshal(execution.ReceiptAttributes)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO gmp_management_execution_receipts (execution_hash, arrived, outcome, external_ref, attributes) VALUES (?, ?, ?, ?, ?)",
		hash, execution.State == campaign.StateArrived, string(execution.Outcome), execution.ReceiptExternalRef, attributes,
	)
	return err
}

func executionHash(key campaign.IdempotencyKey) [sha256.Size]byte {
	return sha256.Sum256([]byte(key))
}

func (l *MySQLExecutionLog) conflict(operation string, started time.Time) error {
	l.logger.Warn("mysql_execution_log_operation", "operation", operation, "result", "rejected", "reason", "conflicting_receipt", "duration_ms", time.Since(started).Milliseconds())
	return campaign.ErrConflictingReceipt
}

func (l *MySQLExecutionLog) failure(operation string, started time.Time, err error) error {
	l.logger.Error("mysql_execution_log_operation", "operation", operation, "result", "failed", "error_type", fmt.Sprintf("%T", err), "duration_ms", time.Since(started).Milliseconds())
	return err
}

func (l *MySQLExecutionLog) success(operation string, started time.Time) {
	l.logger.Info("mysql_execution_log_operation", "operation", operation, "result", "success", "duration_ms", time.Since(started).Milliseconds())
}

func (l *MySQLExecutionLog) replayed(operation string, started time.Time) {
	l.logger.Info("mysql_execution_log_operation", "operation", operation, "result", "replayed", "duration_ms", time.Since(started).Milliseconds())
}
