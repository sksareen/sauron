package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS clipboard_history (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    content      TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text',
    source_app   TEXT,
    bundle_id    TEXT,
    window_title TEXT,
    captured_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS activity_log (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name     TEXT NOT NULL,
    bundle_id    TEXT,
    window_title TEXT,
    started_at   INTEGER NOT NULL,
    ended_at     INTEGER,
    duration_ms  INTEGER
);

CREATE TABLE IF NOT EXISTS context_sessions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_type TEXT NOT NULL,
    focus_score  REAL,
    started_at   INTEGER NOT NULL,
    ended_at     INTEGER,
    app_switches INTEGER NOT NULL DEFAULT 0,
    dominant_app TEXT
);

CREATE TABLE IF NOT EXISTS screenshots (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path    TEXT NOT NULL,
    source_app   TEXT,
    bundle_id    TEXT,
    window_title TEXT,
    captured_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS intent_traces (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    outcome_type           TEXT NOT NULL,
    outcome_detail         TEXT NOT NULL DEFAULT '',
    trace_summary          TEXT NOT NULL DEFAULT '',
    embedding              BLOB,
    activity_window_minutes INTEGER NOT NULL DEFAULT 30,
    started_at             INTEGER NOT NULL,
    completed_at           INTEGER NOT NULL,
    raw_events             TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_clipboard_captured_at ON clipboard_history(captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_started_at   ON activity_log(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at   ON context_sessions(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_traces_completed_at   ON intent_traces(completed_at DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS clipboard_fts USING fts5(
    content,
    source_app,
    window_title,
    content=clipboard_history,
    content_rowid=id
);

CREATE TRIGGER IF NOT EXISTS clipboard_fts_insert
AFTER INSERT ON clipboard_history BEGIN
    INSERT INTO clipboard_fts(rowid, content, source_app, window_title)
    VALUES (new.id, new.content, new.source_app, new.window_title);
END;

CREATE TRIGGER IF NOT EXISTS clipboard_fts_delete
AFTER DELETE ON clipboard_history BEGIN
    INSERT INTO clipboard_fts(clipboard_fts, rowid, content, source_app, window_title)
    VALUES ('delete', old.id, old.content, old.source_app, old.window_title);
END;

CREATE TABLE IF NOT EXISTS experiences (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_intent     TEXT NOT NULL,
    approach        TEXT NOT NULL,
    tools_used      TEXT,
    failure_points  TEXT,
    resolution      TEXT,
    outcome         TEXT NOT NULL CHECK(outcome IN ('success', 'failure', 'partial')),
    tags            TEXT,
    source          TEXT,
    embedding       BLOB,
    created_at      TEXT DEFAULT (datetime('now')),
    updated_at      TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_experiences_outcome    ON experiences(outcome);
CREATE INDEX IF NOT EXISTS idx_experiences_created_at ON experiences(created_at);

CREATE TRIGGER IF NOT EXISTS clipboard_fts_update
AFTER UPDATE ON clipboard_history BEGIN
    INSERT INTO clipboard_fts(clipboard_fts, rowid, content, source_app, window_title)
    VALUES ('delete', old.id, old.content, old.source_app, old.window_title);
    INSERT INTO clipboard_fts(rowid, content, source_app, window_title)
    VALUES (new.id, new.content, new.source_app, new.window_title);
END;
`

// DB wraps the underlying *sql.DB.
type DB struct {
	*sql.DB
}

// DBPath returns the path to the sauron database file.
func DBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".sauron", "sauron.db")
	}
	return filepath.Join(home, ".sauron", "sauron.db")
}

// Open opens (or creates) the sauron SQLite database and runs migrations.
func Open() (*DB, error) {
	dbPath := DBPath()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("creating .sauron dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging sqlite: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("running schema migrations: %w", err)
	}

	return &DB{db}, nil
}

// OpenReadOnly opens an existing database for reading.
// Returns a descriptive error if the database does not exist.
func OpenReadOnly() (*DB, error) {
	dbPath := DBPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("sauron: daemon not running. Run 'sauron start' first")
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite read-only: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sauron: cannot read database: %w", err)
	}

	return &DB{db}, nil
}
