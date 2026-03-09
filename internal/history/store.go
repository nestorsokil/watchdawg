package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"watchdawg/internal/models"
)

// ErrNotFound is returned by QueryCheck when no records exist for the given check name.
var ErrNotFound = errors.New("no history found for check")

// Record is the in-memory representation of a single execution record returned by queries.
type Record struct {
	ID         string
	Timestamp  time.Time
	Healthy    bool
	DurationMs int64
	Error      string
}

// ExecutionStore persists execution records and supports querying them by check name.
type ExecutionStore interface {
	Write(ctx context.Context, check *models.HealthCheck, result *models.CheckResult, retention int) error
	QueryCheck(ctx context.Context, checkName string, limit int) ([]Record, error)
	QueryAll(ctx context.Context, limit int) (map[string][]Record, error)
	Close() error
}

// SQLiteStore implements ExecutionStore using a local SQLite database.
type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

const createTableSQL = `
CREATE TABLE IF NOT EXISTS execution_records (
    id          TEXT    PRIMARY KEY,
    check_name  TEXT    NOT NULL,
    timestamp   INTEGER NOT NULL,
    healthy     INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    error       TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_exec_check_ts
    ON execution_records (check_name, timestamp DESC);
`

// NewSQLiteStore opens (or creates) the SQLite database at cfg.DBPath, runs schema
// migrations, and configures the connection pool. It fails fast if the DB cannot be opened.
func NewSQLiteStore(cfg *models.HistoryConfig, logger *slog.Logger) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", cfg.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteStore{db: db, logger: logger}, nil
}

// Write inserts one execution record and evicts oldest records to stay within retention.
// Both operations run inside a single transaction so the count never transiently exceeds the limit.
func (s *SQLiteStore) Write(ctx context.Context, check *models.HealthCheck, result *models.CheckResult, retention int) error {
	id := uuid.New().String()

	healthy := 0
	if result.Healthy {
		healthy = 1
	}
	timestampNs := result.Timestamp.UnixNano()

	// Consolidate error message: use Error if set; fall back to Message for validation failures
	// (e.g. unexpected HTTP status code sets Message but not Error).
	errMsg := result.Error
	if errMsg == "" && !result.Healthy {
		errMsg = result.Message
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx,
		`INSERT INTO execution_records (id, check_name, timestamp, healthy, duration_ms, error) VALUES (?, ?, ?, ?, ?, ?)`,
		id, check.Name, timestampNs, healthy, result.Duration, errMsg,
	)
	if err != nil {
		return fmt.Errorf("insert record: %w", err)
	}

	// Evict oldest records beyond the retention limit for this check.
	_, err = tx.ExecContext(ctx,
		`DELETE FROM execution_records
		 WHERE check_name = ?
		   AND id NOT IN (
		     SELECT id FROM execution_records
		     WHERE check_name = ?
		     ORDER BY timestamp DESC
		     LIMIT ?
		   )`,
		check.Name, check.Name, retention,
	)
	if err != nil {
		return fmt.Errorf("evict old records: %w", err)
	}

	return tx.Commit()
}

// QueryCheck returns up to limit records for checkName, newest first.
// Returns ErrNotFound if no records exist for that check name.
func (s *SQLiteStore) QueryCheck(ctx context.Context, checkName string, limit int) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, healthy, duration_ms, error
		 FROM execution_records
		 WHERE check_name = ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		checkName, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w %q", ErrNotFound, checkName)
	}
	return records, nil
}

// QueryAll returns up to limit records per check for every check that has history, newest first per check.
func (s *SQLiteStore) QueryAll(ctx context.Context, limit int) (map[string][]Record, error) {
	nameRows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT check_name FROM execution_records ORDER BY check_name`,
	)
	if err != nil {
		return nil, err
	}
	defer nameRows.Close()

	var names []string
	for nameRows.Next() {
		var name string
		if err := nameRows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := nameRows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string][]Record, len(names))
	for _, name := range names {
		records, err := s.QueryCheck(ctx, name, limit)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		result[name] = records
	}
	return result, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// scanRecords reads rows of (id, timestamp, healthy, duration_ms, error) into Record slices.
func scanRecords(rows *sql.Rows) ([]Record, error) {
	var records []Record
	for rows.Next() {
		var r Record
		var tsNano int64
		var healthy int
		if err := rows.Scan(&r.ID, &tsNano, &healthy, &r.DurationMs, &r.Error); err != nil {
			return nil, err
		}
		r.Timestamp = time.Unix(0, tsNano).UTC()
		r.Healthy = healthy == 1
		records = append(records, r)
	}
	return records, rows.Err()
}

