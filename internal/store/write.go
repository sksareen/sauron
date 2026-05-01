package store

import (
	"encoding/json"
	"fmt"
)

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

// InsertVercelLog records a single runtime log line from a Vercel deployment.
// Uses INSERT OR IGNORE on dedupe_hash so restarts of the watcher don't
// double-write the same (timestampInMs, message) pair. Returns (inserted, err).
func InsertVercelLog(db *DB, log *VercelLog, dedupeHash string) (bool, error) {
	const q = `
INSERT OR IGNORE INTO live_vercel_logs
    (deployment_url, level, method, path, status_code, message, dedupe_hash, captured_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(q,
		log.DeploymentURL,
		nilIfEmpty(log.Level),
		nilIfEmpty(log.Method),
		nilIfEmpty(log.Path),
		log.StatusCode,
		log.Message,
		dedupeHash,
		log.CapturedAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert vercel log: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
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

// UpsertProject creates or updates a v2 project and returns its row ID.
func UpsertProject(db *DB, p *Project) (int64, error) {
	const q = `
INSERT INTO projects (project_key, name, kind, root_hint, created_at, updated_at, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_key) DO UPDATE SET
    name = excluded.name,
    kind = excluded.kind,
    root_hint = excluded.root_hint,
    updated_at = excluded.updated_at,
    metadata_json = excluded.metadata_json`
	if p.MetadataJSON == "" {
		p.MetadataJSON = "{}"
	}
	if _, err := db.Exec(q, p.ProjectKey, p.Name, p.Kind, p.RootHint, p.CreatedAt, p.UpdatedAt, p.MetadataJSON); err != nil {
		return 0, fmt.Errorf("upsert project: %w", err)
	}
	var id int64
	if err := db.QueryRow("SELECT id FROM projects WHERE project_key = ?", p.ProjectKey).Scan(&id); err != nil {
		return 0, fmt.Errorf("lookup project: %w", err)
	}
	p.ID = id
	return id, nil
}

// UpsertOpenTask creates or updates a v2 open task and returns its row ID.
func UpsertOpenTask(db *DB, t *OpenTask) (int64, error) {
	const q = `
INSERT INTO open_tasks (
    task_id, project_id, status, goal, last_useful_state, next_action,
    confidence, started_at, updated_at, completed_at, completion_source, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
    project_id = excluded.project_id,
    status = excluded.status,
    goal = excluded.goal,
    last_useful_state = excluded.last_useful_state,
    next_action = excluded.next_action,
    confidence = excluded.confidence,
    updated_at = excluded.updated_at,
    completed_at = excluded.completed_at,
    completion_source = excluded.completion_source,
    metadata_json = excluded.metadata_json`
	if t.MetadataJSON == "" {
		t.MetadataJSON = "{}"
	}
	res, err := db.Exec(q,
		t.TaskID,
		zeroNil(t.ProjectID),
		t.Status,
		t.Goal,
		t.LastUsefulState,
		t.NextAction,
		t.Confidence,
		t.StartedAt,
		t.UpdatedAt,
		zeroNil(t.CompletedAt),
		t.CompletionSource,
		t.MetadataJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert open task: %w", err)
	}
	id, err := res.LastInsertId()
	if err == nil && id != 0 {
		t.ID = id
		return id, nil
	}
	if err := db.QueryRow("SELECT id FROM open_tasks WHERE task_id = ?", t.TaskID).Scan(&id); err != nil {
		return 0, fmt.Errorf("lookup open task: %w", err)
	}
	t.ID = id
	return id, nil
}

// UpdateOpenTaskStatus updates only v2 task lifecycle fields.
func UpdateOpenTaskStatus(db *DB, taskID, status, completionSource string, updatedAt, completedAt int64) error {
	const q = `
UPDATE open_tasks
SET status = ?, updated_at = ?, completed_at = ?, completion_source = ?
WHERE task_id = ?`
	if _, err := db.Exec(q, status, updatedAt, zeroNil(completedAt), completionSource, taskID); err != nil {
		return fmt.Errorf("update open task status: %w", err)
	}
	return nil
}

// InsertHumanTrace inserts a v2 human trace and returns its row ID.
func InsertHumanTrace(db *DB, t *HumanTrace) (int64, error) {
	const q = `
INSERT INTO human_traces (
    trace_id, trace_type, project_id, task_id, trigger_type, status,
    started_at, completed_at, summary, next_action, confidence, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if t.MetadataJSON == "" {
		t.MetadataJSON = "{}"
	}
	res, err := db.Exec(q,
		t.TraceID,
		t.TraceType,
		zeroNil(t.ProjectID),
		t.TaskID,
		t.TriggerType,
		t.Status,
		t.StartedAt,
		t.CompletedAt,
		t.Summary,
		t.NextAction,
		t.Confidence,
		t.MetadataJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("insert human trace: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	t.ID = id
	return id, nil
}

// InsertHumanTraceEvent inserts a redacted v2 evidence pointer.
func InsertHumanTraceEvent(db *DB, e *HumanTraceEvent) (int64, error) {
	const q = `
INSERT INTO human_trace_events (
    trace_id, ts, event_type, source_table, source_id, summary,
    app_name, window_title, artifact_uri, severity, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if e.MetadataJSON == "" {
		e.MetadataJSON = "{}"
	}
	res, err := db.Exec(q,
		e.TraceID,
		e.Ts,
		e.EventType,
		e.SourceTable,
		zeroNil(e.SourceID),
		e.Summary,
		e.AppName,
		e.WindowTitle,
		e.ArtifactURI,
		e.Severity,
		e.MetadataJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("insert human trace event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	e.ID = id
	return id, nil
}

// InsertExperience inserts a new experience record and returns its ID.
func InsertExperience(db *DB, exp *ExperienceRecord) (int64, error) {
	const q = `
INSERT INTO experiences (task_intent, approach, tools_used, failure_points, resolution, outcome, tags, source, embedding)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	toolsJSON, _ := json.Marshal(exp.ToolsUsed)
	failJSON, _ := json.Marshal(exp.FailurePoints)
	tagsJSON, _ := json.Marshal(exp.Tags)

	// Store nil for empty slices.
	var toolsStr, failStr, tagsStr *string
	if len(exp.ToolsUsed) > 0 {
		s := string(toolsJSON)
		toolsStr = &s
	}
	if len(exp.FailurePoints) > 0 {
		s := string(failJSON)
		failStr = &s
	}
	if len(exp.Tags) > 0 {
		s := string(tagsJSON)
		tagsStr = &s
	}

	var embeddingBlob []byte
	if len(exp.Embedding) > 0 {
		embeddingBlob = exp.Embedding
	}

	res, err := db.Exec(q, exp.TaskIntent, exp.Approach, toolsStr, failStr,
		nilIfEmpty(exp.Resolution), exp.Outcome, tagsStr, nilIfEmpty(exp.Source), embeddingBlob)
	if err != nil {
		return 0, fmt.Errorf("insert experience: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func zeroNil(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
