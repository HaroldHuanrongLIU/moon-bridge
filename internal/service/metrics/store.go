// Package metrics provides SQLite-backed persistence for per-request usage
// metrics, enabling historical query and analysis beyond the in-memory
// session stats.
package metrics

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultPath is the default SQLite database file name, resolved relative
// to the config directory.
const DefaultPath = "metrics.db"

// Record represents a single request metric row.
type Record struct {
	ID            int64         `json:"id"`
	Timestamp     time.Time     `json:"timestamp"`
	Model         string        `json:"model"`          // request model alias
	ActualModel   string        `json:"actual_model"`   // upstream model name
	InputTokens   int64         `json:"input_tokens"`
	OutputTokens  int64         `json:"output_tokens"`
	CacheCreation int64         `json:"cache_creation"`
	CacheRead     int64         `json:"cache_read"`
	Cost          float64       `json:"cost"`              // RMB
	ResponseTime  time.Duration `json:"response_time"`     // request duration
	Status        string        `json:"status"`            // "success" or "error"
	ErrorMessage  string        `json:"error_message,omitempty"`
}

// QueryOptions controls filtering and ordering for Record queries.
type QueryOptions struct {
	Limit    int
	Offset   int
	Since    time.Time // inclusive; zero = no lower bound
	Until    time.Time // exclusive; zero = no upper bound
	Model    string    // filter by model alias; empty = all
	Status   string    // filter by status; empty = all
	OrderAsc bool      // false = DESC (newest first)
}

// Store is a SQLite-backed metrics store.
type Store struct {
	db *sql.DB
}

// NewStore opens or creates a SQLite database at dbPath and ensures the
// metrics table exists. Returns nil, nil when dbPath is empty (disabled).
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open metrics db: %w", err)
	}
	// WAL mode for better concurrent read/write performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	// Busy timeout: retry up to 5s when the database is locked by another writer.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	// Limit to a single connection to avoid "database is locked" across goroutines.
	db.SetMaxOpenConns(1)
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create metrics schema: %w", err)
	}
	return &Store{db: db}, nil
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS request_metrics (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp       TEXT    NOT NULL,
			model           TEXT    NOT NULL DEFAULT '',
			actual_model    TEXT    NOT NULL DEFAULT '',
			input_tokens    INTEGER NOT NULL DEFAULT 0,
			output_tokens   INTEGER NOT NULL DEFAULT 0,
			cache_creation  INTEGER NOT NULL DEFAULT 0,
			cache_read      INTEGER NOT NULL DEFAULT 0,
			cost            REAL    NOT NULL DEFAULT 0.0,
			response_time_ms INTEGER NOT NULL DEFAULT 0,
			status          TEXT    NOT NULL DEFAULT 'success',
			error_message   TEXT    NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON request_metrics(timestamp);
		CREATE INDEX IF NOT EXISTS idx_metrics_model      ON request_metrics(model);
		CREATE INDEX IF NOT EXISTS idx_metrics_status     ON request_metrics(status);
	`)
	return err
}

// Record inserts a request metric row.
func (s *Store) Record(r Record) error {
	if s == nil || s.db == nil {
		return nil
	}
	const query = `
		INSERT INTO request_metrics
			(timestamp, model, actual_model, input_tokens, output_tokens,
			 cache_creation, cache_read, cost, response_time_ms, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	ms := r.ResponseTime.Milliseconds()
	ts := r.Timestamp.UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(query,
		ts, r.Model, r.ActualModel,
		r.InputTokens, r.OutputTokens, r.CacheCreation, r.CacheRead,
		r.Cost, ms, r.Status, r.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("insert metrics record: %w", err)
	}
	return nil
}

// Query retrieves request metrics matching the given options.
func (s *Store) Query(opts QueryOptions) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	var whereClauses []string
	var args []any

	if !opts.Since.IsZero() {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}
	if !opts.Until.IsZero() {
		whereClauses = append(whereClauses, "timestamp < ?")
		args = append(args, opts.Until.UTC().Format(time.RFC3339Nano))
	}
	if opts.Model != "" {
		whereClauses = append(whereClauses, "model = ?")
		args = append(args, opts.Model)
	}
	if opts.Status != "" {
		whereClauses = append(whereClauses, "status = ?")
		args = append(args, opts.Status)
	}

	order := "DESC"
	if opts.OrderAsc {
		order = "ASC"
	}

	query := "SELECT id, timestamp, model, actual_model, input_tokens, output_tokens, " +
		"cache_creation, cache_read, cost, response_time_ms, status, error_message " +
		"FROM request_metrics"
	if len(whereClauses) > 0 {
		query += " WHERE " + joinWhere(whereClauses)
	}
	query += " ORDER BY id " + order + " LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		var ts string
		var ms int64
		err := rows.Scan(
			&r.ID, &ts, &r.Model, &r.ActualModel,
			&r.InputTokens, &r.OutputTokens, &r.CacheCreation, &r.CacheRead,
			&r.Cost, &ms, &r.Status, &r.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("scan metrics row: %w", err)
		}
		r.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		r.ResponseTime = time.Duration(ms) * time.Millisecond
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metrics rows: %w", err)
	}
	return records, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// joinWhere joins non-empty WHERE clauses with " AND ".
func joinWhere(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += " AND "
		}
		result += c
	}
	return result
}
