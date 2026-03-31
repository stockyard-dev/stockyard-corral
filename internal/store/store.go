package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection.
type DB struct {
	conn *sql.DB
}

// Open creates or opens a SQLite database in the given data directory.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "corral.db")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.Exec("PRAGMA foreign_keys=ON")
	conn.SetMaxOpenConns(4)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	log.Printf("[store] database opened at %s", dbPath)
	return db, nil
}

// Conn returns the underlying sql.DB.
func (db *DB) Conn() *sql.DB { return db.conn }

// Close closes the database connection.
func (db *DB) Close() error { return db.conn.Close() }

func (db *DB) migrate() error {
	_, err := db.conn.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS endpoints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
    method TEXT NOT NULL,
    path TEXT DEFAULT '/',
    headers_json TEXT DEFAULT '{}',
    body TEXT DEFAULT '',
    body_size INTEGER DEFAULT 0,
    content_type TEXT DEFAULT '',
    source_ip TEXT DEFAULT '',
    received_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_endpoint ON events(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_events_received ON events(received_at);

CREATE TABLE IF NOT EXISTS forwards (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
    target_url TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    retry_count INTEGER DEFAULT 3,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_forwards_endpoint ON forwards(endpoint_id);

CREATE TABLE IF NOT EXISTS deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL REFERENCES events(id),
    target_url TEXT NOT NULL,
    status_code INTEGER DEFAULT 0,
    success INTEGER DEFAULT 0,
    error_message TEXT DEFAULT '',
    latency_ms INTEGER DEFAULT 0,
    attempt INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_deliveries_event ON deliveries(event_id);
`

// GenerateID creates a random ID with the given prefix.
func GenerateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
