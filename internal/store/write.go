package store

import "fmt"

// InsertClipboard inserts a new clipboard capture.
func InsertClipboard(db *DB, content, contentType, sourceApp, bundleID, windowTitle string) error {
	const q = `
INSERT INTO clipboard_history (content, content_type, source_app, bundle_id, window_title, captured_at)
VALUES (?, ?, ?, ?, ?, strftime('%s','now'))`
	_, err := db.Exec(q, content, contentType, sourceApp, bundleID, windowTitle)
	if err != nil {
		return fmt.Errorf("insert clipboard: %w", err)
	}
	return nil
}

// InsertActivity starts a new activity entry and returns its row ID.
func InsertActivity(db *DB, appName, bundleID, windowTitle string, startedAt int64) (int64, error) {
	const q = `
INSERT INTO activity_log (app_name, bundle_id, window_title, started_at)
VALUES (?, ?, ?, ?)`
	res, err := db.Exec(q, appName, bundleID, windowTitle, startedAt)
	if err != nil {
		return 0, fmt.Errorf("insert activity: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// EndActivity closes an activity entry and sets duration.
func EndActivity(db *DB, id int64, endedAt int64) error {
	const q = `
UPDATE activity_log
SET ended_at    = ?,
    duration_ms = (? - started_at) * 1000
WHERE id = ?`
	_, err := db.Exec(q, endedAt, endedAt, id)
	if err != nil {
		return fmt.Errorf("end activity: %w", err)
	}
	return nil
}

// InsertSession records a classified context session.
func InsertSession(db *DB, sessionType string, focusScore float64, startedAt int64, appSwitches int, dominantApp string) error {
	const q = `
INSERT INTO context_sessions (session_type, focus_score, started_at, app_switches, dominant_app)
VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(q, sessionType, focusScore, startedAt, appSwitches, dominantApp)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// InsertScreenshot records a captured screenshot path.
func InsertScreenshot(db *DB, filePath, sourceApp, bundleID, windowTitle string, capturedAt int64) error {
	const q = `
INSERT INTO screenshots (file_path, source_app, bundle_id, window_title, captured_at)
VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(q, filePath, sourceApp, bundleID, windowTitle, capturedAt)
	if err != nil {
		return fmt.Errorf("insert screenshot: %w", err)
	}
	return nil
}

// InsertIntentTrace records a new intent trace.
func InsertIntentTrace(db *DB, t *IntentTrace) error {
	const q = `
INSERT INTO intent_traces (outcome_type, outcome_detail, trace_summary, embedding, activity_window_minutes, started_at, completed_at, raw_events)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(q, t.OutcomeType, t.OutcomeDetail, t.TraceSummary,
		t.Embedding, t.ActivityWindowMinutes, t.StartedAt, t.CompletedAt, t.RawEvents)
	if err != nil {
		return fmt.Errorf("insert intent trace: %w", err)
	}
	return nil
}
