package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// StoredEvent represents an event in storage (copied from ebu)
type StoredEvent struct {
	Position  int64           `json:"position"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

// SQLiteStore implements EventStore using SQLite
type SQLiteStore struct {
	db              *sql.DB
	mu              sync.RWMutex
	saveStmt        *sql.Stmt
	loadStmt        *sql.Stmt
	loadRangeStmt   *sql.Stmt
	positionStmt    *sql.Stmt
	saveSubStmt     *sql.Stmt
	loadSubStmt     *sql.Stmt
}

// NewSQLiteStore creates a new SQLite-based event store
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Connection pool settings for high throughput
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Production-ready SQLite performance tuning
	pragmas := []string{
		"PRAGMA journal_mode=WAL",           // Better concurrency
		"PRAGMA synchronous=NORMAL",         // Good balance of safety/performance
		"PRAGMA cache_size=-64000",          // 64MB cache
		"PRAGMA busy_timeout=5000",          // 5s busy timeout
		"PRAGMA wal_autocheckpoint=1000",    // Checkpoint every 1000 pages
		"PRAGMA temp_store=MEMORY",          // Keep temp tables in memory
		"PRAGMA mmap_size=268435456",        // 256MB mmap
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("execute %s: %w", pragma, err)
		}
	}

	// Create tables
	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("create tables: %w", err)
	}

	// Prepare statements for better performance
	store := &SQLiteStore{db: db}
	if err := store.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}

	return store, nil
}

func (s *SQLiteStore) prepareStatements() error {
	var err error

	s.saveStmt, err = s.db.Prepare("INSERT INTO events (type, data, timestamp) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare save: %w", err)
	}

	s.loadStmt, err = s.db.Prepare("SELECT position, type, data, timestamp FROM events WHERE position >= ? ORDER BY position LIMIT ?")
	if err != nil {
		return fmt.Errorf("prepare load: %w", err)
	}

	s.loadRangeStmt, err = s.db.Prepare("SELECT position, type, data, timestamp FROM events WHERE position >= ? AND position <= ? ORDER BY position")
	if err != nil {
		return fmt.Errorf("prepare load range: %w", err)
	}

	s.positionStmt, err = s.db.Prepare("SELECT MAX(position) FROM events")
	if err != nil {
		return fmt.Errorf("prepare position: %w", err)
	}

	s.saveSubStmt, err = s.db.Prepare("INSERT OR REPLACE INTO subscriptions (subscription_id, position) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare save subscription: %w", err)
	}

	s.loadSubStmt, err = s.db.Prepare("SELECT position FROM subscriptions WHERE subscription_id = ?")
	if err != nil {
		return fmt.Errorf("prepare load subscription: %w", err)
	}

	return nil
}

func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		position INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL
	);

	-- Composite index for type-based queries with position range
	CREATE INDEX IF NOT EXISTS idx_events_type_position ON events(type, position);

	-- Index for timestamp-based queries (useful for time-range replays)
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

	-- Covering index for common replay patterns (avoids table lookup)
	CREATE INDEX IF NOT EXISTS idx_events_position_type ON events(position, type) WHERE position > 0;

	CREATE TABLE IF NOT EXISTS subscriptions (
		subscription_id TEXT PRIMARY KEY,
		position INTEGER NOT NULL
	);

	-- Analyze tables for query optimizer
	ANALYZE;
	`

	_, err := db.Exec(schema)
	return err
}

// Save implements EventStore.Save
func (s *SQLiteStore) Save(ctx context.Context, event *StoredEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.saveStmt.ExecContext(ctx, event.Type, event.Data, event.Timestamp)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	position, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	event.Position = position
	return nil
}

// SaveBatch saves multiple events in a single transaction for better performance
func (s *SQLiteStore) SaveBatch(ctx context.Context, events []*StoredEvent) error {
	if len(events) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt := tx.StmtContext(ctx, s.saveStmt)

	for _, event := range events {
		result, err := stmt.ExecContext(ctx, event.Type, event.Data, event.Timestamp)
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}

		position, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get last insert id: %w", err)
		}

		event.Position = position
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// Load implements EventStore.Load with pagination for large datasets
// For production use with large event counts, use LoadStream instead
func (s *SQLiteStore) Load(ctx context.Context, from, to int64) ([]*StoredEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rows *sql.Rows
	var err error

	if to == -1 {
		// Default limit to prevent OOM on huge datasets
		rows, err = s.loadStmt.QueryContext(ctx, from, 10000)
	} else {
		rows, err = s.loadRangeStmt.QueryContext(ctx, from, to)
	}

	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	// Pre-allocate slice with reasonable capacity
	events := make([]*StoredEvent, 0, 1000)
	for rows.Next() {
		var event StoredEvent
		if err := rows.Scan(&event.Position, &event.Type, &event.Data, &event.Timestamp); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return events, nil
}

// LoadStream loads events in batches and calls handler for each batch
// This prevents loading huge datasets into memory at once
func (s *SQLiteStore) LoadStream(ctx context.Context, from int64, batchSize int, handler func([]*StoredEvent) error) error {
	if batchSize <= 0 {
		batchSize = 1000
	}

	position := from
	for {
		s.mu.RLock()
		rows, err := s.loadStmt.QueryContext(ctx, position, batchSize)
		s.mu.RUnlock()

		if err != nil {
			return fmt.Errorf("query events: %w", err)
		}

		batch := make([]*StoredEvent, 0, batchSize)
		for rows.Next() {
			var event StoredEvent
			if err := rows.Scan(&event.Position, &event.Type, &event.Data, &event.Timestamp); err != nil {
				rows.Close()
				return fmt.Errorf("scan event: %w", err)
			}
			batch = append(batch, &event)
		}
		rows.Close()

		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate events: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		if err := handler(batch); err != nil {
			return fmt.Errorf("handle batch: %w", err)
		}

		// If we got less than batchSize, we're done
		if len(batch) < batchSize {
			break
		}

		// Move to next batch
		position = batch[len(batch)-1].Position + 1
	}

	return nil
}

// GetPosition implements EventStore.GetPosition
func (s *SQLiteStore) GetPosition(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var position sql.NullInt64
	err := s.positionStmt.QueryRowContext(ctx).Scan(&position)
	if err != nil {
		return 0, fmt.Errorf("get max position: %w", err)
	}

	if !position.Valid {
		return 0, nil
	}

	return position.Int64, nil
}

// SaveSubscriptionPosition implements EventStore.SaveSubscriptionPosition
func (s *SQLiteStore) SaveSubscriptionPosition(ctx context.Context, subscriptionID string, position int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.saveSubStmt.ExecContext(ctx, subscriptionID, position)
	if err != nil {
		return fmt.Errorf("save subscription position: %w", err)
	}

	return nil
}

// LoadSubscriptionPosition implements EventStore.LoadSubscriptionPosition
func (s *SQLiteStore) LoadSubscriptionPosition(ctx context.Context, subscriptionID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var position sql.NullInt64
	err := s.loadSubStmt.QueryRowContext(ctx, subscriptionID).Scan(&position)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load subscription position: %w", err)
	}

	if !position.Valid {
		return 0, nil
	}

	return position.Int64, nil
}

// Close closes the database connection and prepared statements
func (s *SQLiteStore) Close() error {
	// Close prepared statements
	if s.saveStmt != nil {
		s.saveStmt.Close()
	}
	if s.loadStmt != nil {
		s.loadStmt.Close()
	}
	if s.loadRangeStmt != nil {
		s.loadRangeStmt.Close()
	}
	if s.positionStmt != nil {
		s.positionStmt.Close()
	}
	if s.saveSubStmt != nil {
		s.saveSubStmt.Close()
	}
	if s.loadSubStmt != nil {
		s.loadSubStmt.Close()
	}

	return s.db.Close()
}
