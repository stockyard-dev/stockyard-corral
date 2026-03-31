package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct{ conn *sql.DB }

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	conn, err := sql.Open("sqlite", filepath.Join(dataDir, "corral.db"))
	if err != nil {
		return nil, err
	}
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.SetMaxOpenConns(4)
	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) Conn() *sql.DB { return db.conn }
func (db *DB) Close() error  { return db.conn.Close() }

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
CREATE TABLE IF NOT EXISTS endpoints (
    id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT DEFAULT '',
    forward_url TEXT DEFAULT '', enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY, endpoint_id TEXT NOT NULL, method TEXT DEFAULT 'POST',
    path TEXT DEFAULT '/', headers_json TEXT DEFAULT '{}', body TEXT DEFAULT '',
    body_size INTEGER DEFAULT 0, content_type TEXT DEFAULT '', source_ip TEXT DEFAULT '',
    received_at TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_ep ON events(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_events_time ON events(received_at);
CREATE TABLE IF NOT EXISTS replays (
    id TEXT PRIMARY KEY, event_id TEXT NOT NULL, target_url TEXT NOT NULL,
    status_code INTEGER DEFAULT 0, response_body TEXT DEFAULT '',
    latency_ms INTEGER DEFAULT 0, error TEXT DEFAULT '',
    replayed_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS forward_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT, endpoint_id TEXT NOT NULL,
    target_url TEXT NOT NULL, filter_header TEXT DEFAULT '', filter_value TEXT DEFAULT '',
    retry_count INTEGER DEFAULT 3, enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS forward_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT, rule_id INTEGER, event_id TEXT,
    status_code INTEGER DEFAULT 0, latency_ms INTEGER DEFAULT 0,
    attempt INTEGER DEFAULT 1, error TEXT DEFAULT '',
    forwarded_at TEXT DEFAULT (datetime('now'))
);`)
	return err
}

type Endpoint struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Desc       string `json:"description"`
	ForwardURL string `json:"forward_url"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  string `json:"created_at"`
	Events     int    `json:"event_count"`
}

func (db *DB) CreateEndpoint(name, desc string) (*Endpoint, error) {
	id := "ep_" + genID(8)
	_, err := db.conn.Exec("INSERT INTO endpoints (id,name,description) VALUES (?,?,?)", id, name, desc)
	if err != nil {
		return nil, err
	}
	return &Endpoint{ID: id, Name: name, Desc: desc, Enabled: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (db *DB) ListEndpoints() ([]Endpoint, error) {
	rows, err := db.conn.Query(`SELECT e.id, e.name, e.description, e.forward_url, e.enabled, e.created_at,
		(SELECT COUNT(*) FROM events WHERE endpoint_id=e.id) FROM endpoints e ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Endpoint
	for rows.Next() {
		var ep Endpoint
		var en int
		rows.Scan(&ep.ID, &ep.Name, &ep.Desc, &ep.ForwardURL, &en, &ep.CreatedAt, &ep.Events)
		ep.Enabled = en == 1
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (db *DB) GetEndpoint(id string) (*Endpoint, error) {
	var ep Endpoint
	var en int
	err := db.conn.QueryRow(`SELECT id,name,description,forward_url,enabled,created_at,
		(SELECT COUNT(*) FROM events WHERE endpoint_id=endpoints.id) FROM endpoints WHERE id=?`, id).
		Scan(&ep.ID, &ep.Name, &ep.Desc, &ep.ForwardURL, &en, &ep.CreatedAt, &ep.Events)
	if err != nil {
		return nil, err
	}
	ep.Enabled = en == 1
	return &ep, nil
}

func (db *DB) DeleteEndpoint(id string) error {
	db.conn.Exec("DELETE FROM events WHERE endpoint_id=?", id)
	db.conn.Exec("DELETE FROM forward_rules WHERE endpoint_id=?", id)
	_, err := db.conn.Exec("DELETE FROM endpoints WHERE id=?", id)
	return err
}

type Event struct {
	ID         string `json:"id"`
	EndpointID string `json:"endpoint_id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Headers    string `json:"headers"`
	Body       string `json:"body"`
	BodySize   int    `json:"body_size"`
	CType      string `json:"content_type"`
	SourceIP   string `json:"source_ip"`
	ReceivedAt string `json:"received_at"`
}

func (db *DB) RecordEvent(epID, method, path, headers, body, ctype, ip string) (*Event, error) {
	id := "evt_" + genID(10)
	_, err := db.conn.Exec(`INSERT INTO events (id,endpoint_id,method,path,headers_json,body,body_size,content_type,source_ip)
		VALUES (?,?,?,?,?,?,?,?,?)`, id, epID, method, path, headers, body, len(body), ctype, ip)
	if err != nil {
		return nil, err
	}
	return &Event{ID: id, EndpointID: epID, Method: method, Path: path, Headers: headers,
		Body: body, BodySize: len(body), CType: ctype, SourceIP: ip,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (db *DB) ListEvents(epID string, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.conn.Query(`SELECT id,endpoint_id,method,path,headers_json,body,body_size,content_type,source_ip,received_at
		FROM events WHERE endpoint_id=? ORDER BY received_at DESC LIMIT ?`, epID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		rows.Scan(&e.ID, &e.EndpointID, &e.Method, &e.Path, &e.Headers, &e.Body, &e.BodySize, &e.CType, &e.SourceIP, &e.ReceivedAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (db *DB) GetEvent(id string) (*Event, error) {
	var e Event
	err := db.conn.QueryRow(`SELECT id,endpoint_id,method,path,headers_json,body,body_size,content_type,source_ip,received_at
		FROM events WHERE id=?`, id).Scan(&e.ID, &e.EndpointID, &e.Method, &e.Path, &e.Headers, &e.Body, &e.BodySize, &e.CType, &e.SourceIP, &e.ReceivedAt)
	return &e, err
}

func (db *DB) RecordReplay(eventID, target string, status int, respBody string, latency int, errMsg string) (string, error) {
	id := "rpl_" + genID(8)
	_, err := db.conn.Exec(`INSERT INTO replays (id,event_id,target_url,status_code,response_body,latency_ms,error)
		VALUES (?,?,?,?,?,?,?)`, id, eventID, target, status, respBody, latency, errMsg)
	return id, err
}

type ForwardRule struct {
	ID       int    `json:"id"`
	EpID     string `json:"endpoint_id"`
	Target   string `json:"target_url"`
	FiltHdr  string `json:"filter_header"`
	FiltVal  string `json:"filter_value"`
	Retries  int    `json:"retry_count"`
	Enabled  bool   `json:"enabled"`
}

func (db *DB) ListForwardRules(epID string) ([]ForwardRule, error) {
	rows, err := db.conn.Query(`SELECT id,endpoint_id,target_url,filter_header,filter_value,retry_count,enabled
		FROM forward_rules WHERE endpoint_id=? AND enabled=1`, epID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ForwardRule
	for rows.Next() {
		var r ForwardRule
		var en int
		rows.Scan(&r.ID, &r.EpID, &r.Target, &r.FiltHdr, &r.FiltVal, &r.Retries, &en)
		r.Enabled = en == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) CreateForwardRule(epID, target, filtHdr, filtVal string, retries int) error {
	_, err := db.conn.Exec(`INSERT INTO forward_rules (endpoint_id,target_url,filter_header,filter_value,retry_count)
		VALUES (?,?,?,?,?)`, epID, target, filtHdr, filtVal, retries)
	return err
}

func (db *DB) LogForward(ruleID int, eventID string, status, latency, attempt int, errMsg string) {
	db.conn.Exec(`INSERT INTO forward_log (rule_id,event_id,status_code,latency_ms,attempt,error)
		VALUES (?,?,?,?,?,?)`, ruleID, eventID, status, latency, attempt, errMsg)
}

func (db *DB) Stats() map[string]any {
	var ep, ev, rp, fr int
	db.conn.QueryRow("SELECT COUNT(*) FROM endpoints").Scan(&ep)
	db.conn.QueryRow("SELECT COUNT(*) FROM events").Scan(&ev)
	db.conn.QueryRow("SELECT COUNT(*) FROM replays").Scan(&rp)
	db.conn.QueryRow("SELECT COUNT(*) FROM forward_rules WHERE enabled=1").Scan(&fr)
	return map[string]any{"endpoints": ep, "events": ev, "replays": rp, "forward_rules": fr}
}

func (db *DB) Cleanup(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	res, err := db.conn.Exec("DELETE FROM events WHERE received_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func genID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GenerateID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
