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

CREATE TABLE IF NOT EXISTS live_vercel_logs (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    deployment_url TEXT NOT NULL,
    level          TEXT,
    method         TEXT,
    path           TEXT,
    status_code    INTEGER,
    message        TEXT NOT NULL,
    dedupe_hash    TEXT NOT NULL,
    captured_at    INTEGER NOT NULL,
    UNIQUE(dedupe_hash)
);

CREATE TABLE IF NOT EXISTS projects (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    project_key   TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL DEFAULT 'inferred',
    root_hint     TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS open_tasks (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id           TEXT NOT NULL UNIQUE,
    project_id        INTEGER,
    status            TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'paused', 'completed', 'abandoned')),
    goal              TEXT NOT NULL DEFAULT '',
    last_useful_state TEXT NOT NULL DEFAULT '',
    next_action       TEXT NOT NULL DEFAULT '',
    confidence        REAL NOT NULL DEFAULT 0,
    started_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    completed_at      INTEGER,
    completion_source TEXT NOT NULL DEFAULT '',
    metadata_json     TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS human_traces (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id      TEXT NOT NULL UNIQUE,
    trace_type    TEXT NOT NULL CHECK(trace_type IN ('debugging', 'interruption', 'reentry', 'completion')),
    project_id    INTEGER,
    task_id       TEXT NOT NULL DEFAULT '',
    trigger_type  TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'unknown',
    started_at    INTEGER NOT NULL,
    completed_at  INTEGER NOT NULL,
    summary       TEXT NOT NULL DEFAULT '',
    next_action   TEXT NOT NULL DEFAULT '',
    confidence    REAL NOT NULL DEFAULT 0,
    metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS human_trace_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id      TEXT NOT NULL,
    ts            INTEGER NOT NULL,
    event_type    TEXT NOT NULL,
    source_table  TEXT NOT NULL DEFAULT '',
    source_id     INTEGER,
    summary       TEXT NOT NULL DEFAULT '',
    app_name      TEXT NOT NULL DEFAULT '',
    window_title  TEXT NOT NULL DEFAULT '',
    artifact_uri  TEXT NOT NULL DEFAULT '',
    severity      TEXT NOT NULL DEFAULT 'info',
    metadata_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_clipboard_captured_at ON clipboard_history(captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_started_at   ON activity_log(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at   ON context_sessions(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_traces_completed_at   ON intent_traces(completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_vercel_captured_at    ON live_vercel_logs(captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_projects_project_key  ON projects(project_key);
CREATE INDEX IF NOT EXISTS idx_open_tasks_status     ON open_tasks(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_open_tasks_project    ON open_tasks(project_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_human_traces_type     ON human_traces(trace_type, completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_human_traces_task     ON human_traces(task_id, completed_at DESC);
CREATE INDEX IF NOT EXISTS idx_human_trace_events    ON human_trace_events(trace_id, ts ASC);

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

CREATE TABLE IF NOT EXISTS hints (
    id             TEXT PRIMARY KEY,
    label          TEXT NOT NULL DEFAULT '',
    confidence     REAL NOT NULL DEFAULT 0,
    weight         REAL NOT NULL DEFAULT 1.0,
    status         TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused','completed','abandoned')),
    dominant_app   TEXT NOT NULL DEFAULT '',
    window_pattern TEXT NOT NULL DEFAULT '',
    merge_group_id TEXT,
    embedding      BLOB,
    started_at     INTEGER NOT NULL,
    last_active_at INTEGER NOT NULL,
    labelled_at    INTEGER NOT NULL DEFAULT 0,
    evidence_count INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS hint_evidence (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    hint_id      TEXT NOT NULL REFERENCES hints(id),
    source_table TEXT NOT NULL,
    source_id    INTEGER NOT NULL,
    ts           INTEGER NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    app_name     TEXT NOT NULL DEFAULT '',
    window_title TEXT NOT NULL DEFAULT '',
    severity     TEXT NOT NULL DEFAULT 'info'
);

CREATE INDEX IF NOT EXISTS idx_hints_status        ON hints(status, weight DESC);
CREATE INDEX IF NOT EXISTS idx_hints_last_active   ON hints(last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_hint_evidence_hint  ON hint_evidence(hint_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_hint_evidence_ts    ON hint_evidence(ts DESC);
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
