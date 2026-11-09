package management

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var ErrColoringKeyCollision = errors.New("management: coloring key hash collision")

func (s *MySQLStore) LookupColoring(ctx context.Context, assignmentID experimentation.AssignmentID, key experimentation.AssignmentKey) (experimentation.ColoringRecord, bool, error) {
	started := time.Now()
	if assignmentID == "" || key == "" {
		return experimentation.ColoringRecord{}, false, errors.New("management: assignment ID and coloring key are required")
	}
	assignmentHash := sha256.Sum256([]byte(assignmentID))
	keyHash := sha256.Sum256([]byte(key))
	var (
		storedAssignment string
		storedKey        string
		bucketValue      int
		algorithm        string
	)
	err := s.db.QueryRowContext(ctx,
		"SELECT assignment_id, assignment_key, bucket, algorithm FROM gmp_management_experiment_colorings WHERE assignment_hash = ? AND key_hash = ?",
		assignmentHash[:], keyHash[:],
	).Scan(&storedAssignment, &storedKey, &bucketValue, &algorithm)
	if errors.Is(err, sql.ErrNoRows) {
		s.logger.Info("mysql_coloring_operation", "operation", "lookup", "result", "not_found", "duration_ms", time.Since(started).Milliseconds())
		return experimentation.ColoringRecord{}, false, nil
	}
	if err != nil {
		return experimentation.ColoringRecord{}, false, s.coloringFailure("lookup", started, err)
	}
	if storedAssignment != string(assignmentID) || storedKey != string(key) {
		return experimentation.ColoringRecord{}, false, s.coloringFailure("lookup", started, ErrColoringKeyCollision)
	}
	bucket, err := experimentation.NewBucketNumber(bucketValue)
	if err != nil || algorithm == "" {
		return experimentation.ColoringRecord{}, false, s.coloringFailure("lookup", started, errors.New("management: persisted coloring record is invalid"))
	}
	s.logger.Info("mysql_coloring_operation", "operation", "lookup", "result", "success", "duration_ms", time.Since(started).Milliseconds())
	return experimentation.ColoringRecord{Bucket: bucket, Algorithm: experimentation.AlgorithmVersion(algorithm)}, true, nil
}

func (s *MySQLStore) RecordColoringFirst(ctx context.Context, assignmentID experimentation.AssignmentID, key experimentation.AssignmentKey, record experimentation.ColoringRecord) (experimentation.ColoringRecord, error) {
	started := time.Now()
	if assignmentID == "" || key == "" || record.Bucket.IsZero() || record.Algorithm == "" {
		return experimentation.ColoringRecord{}, errors.New("management: assignment ID, coloring key, bucket and algorithm are required")
	}
	assignmentHash := sha256.Sum256([]byte(assignmentID))
	keyHash := sha256.Sum256([]byte(key))
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO gmp_management_experiment_colorings (assignment_hash, assignment_id, key_hash, assignment_key, bucket, algorithm) VALUES (?, ?, ?, ?, ?, ?)",
		assignmentHash[:], string(assignmentID), keyHash[:], string(key), record.Bucket.Value(), string(record.Algorithm),
	)
	if err == nil {
		s.logger.Info("mysql_coloring_operation", "operation", "record_first", "result", "inserted", "duration_ms", time.Since(started).Milliseconds())
		return record, nil
	}
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
		return experimentation.ColoringRecord{}, s.coloringFailure("record_first", started, err)
	}
	winner, found, readErr := s.LookupColoring(ctx, assignmentID, key)
	if readErr != nil {
		return experimentation.ColoringRecord{}, s.coloringFailure("record_first", started, readErr)
	}
	if !found {
		return experimentation.ColoringRecord{}, s.coloringFailure("record_first", started, errors.New("management: coloring conflict disappeared before winner read"))
	}
	s.logger.Info("mysql_coloring_operation", "operation", "record_first", "result", "existing_winner", "duration_ms", time.Since(started).Milliseconds())
	return winner, nil
}

func (s *MySQLStore) coloringFailure(operation string, started time.Time, err error) error {
	s.logger.Error("mysql_coloring_operation", "operation", operation, "result", "failed", "error_type", fmt.Sprintf("%T", err), "duration_ms", time.Since(started).Milliseconds())
	if errors.Is(err, ErrColoringKeyCollision) {
		return ErrColoringKeyCollision
	}
	return err
}

var _ interface {
	LookupColoring(context.Context, experimentation.AssignmentID, experimentation.AssignmentKey) (experimentation.ColoringRecord, bool, error)
	RecordColoringFirst(context.Context, experimentation.AssignmentID, experimentation.AssignmentKey, experimentation.ColoringRecord) (experimentation.ColoringRecord, error)
} = (*MySQLStore)(nil)
