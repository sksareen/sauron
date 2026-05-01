package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// GetRecentClipboard returns the most recent n clipboard items.
func GetRecentClipboard(db *DB, n int) ([]ClipboardItem, error) {
	const q = `
SELECT id, content, content_type, COALESCE(source_app,''), COALESCE(bundle_id,''),
       COALESCE(window_title,''), captured_at
FROM clipboard_history
ORDER BY captured_at DESC
LIMIT ?`

	rows, err := db.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("query clipboard: %w", err)
	}
	defer rows.Close()

	var items []ClipboardItem
	for rows.Next() {
		var item ClipboardItem
		if err := rows.Scan(&item.ID, &item.Content, &item.ContentType,
			&item.SourceApp, &item.BundleID, &item.WindowTitle, &item.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan clipboard row: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetActivity returns activity entries from the last h hours.
func GetActivity(db *DB, hours float64) ([]ActivityEntry, error) {
	const q = `
SELECT id, app_name, COALESCE(bundle_id,''), COALESCE(window_title,''),
       started_at, COALESCE(ended_at,0), COALESCE(duration_ms,0)
FROM activity_log
WHERE started_at >= strftime('%s','now') - ?
ORDER BY started_at DESC`

	rows, err := db.Query(q, int64(hours*3600))
	if err != nil {
		return nil, fmt.Errorf("query activity: %w", err)
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.ID, &e.AppName, &e.BundleID, &e.WindowTitle,
			&e.StartedAt, &e.EndedAt, &e.DurationMs); err != nil {
			return nil, fmt.Errorf("scan activity row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetCurrentSession returns the most recent context session.
func GetCurrentSession(db *DB) (*ContextSession, error) {
	const q = `
SELECT id, session_type, COALESCE(focus_score,0), started_at,
       COALESCE(ended_at,0), COALESCE(app_switches,0), COALESCE(dominant_app,'')
FROM context_sessions
ORDER BY started_at DESC
LIMIT 1`

	var s ContextSession
	err := db.QueryRow(q).Scan(&s.ID, &s.SessionType, &s.FocusScore,
		&s.StartedAt, &s.EndedAt, &s.AppSwitches, &s.DominantApp)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query current session: %w", err)
	}
	return &s, nil
}

// GetRecentTraces returns the most recent n intent traces.
func GetRecentTraces(db *DB, n int) ([]IntentTrace, error) {
	const q = `
SELECT id, outcome_type, COALESCE(outcome_detail,''), COALESCE(trace_summary,''),
       activity_window_minutes, started_at, completed_at, COALESCE(raw_events,'{}')
FROM intent_traces
ORDER BY completed_at DESC
LIMIT ?`

	rows, err := db.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("query traces: %w", err)
	}
	defer rows.Close()

	var traces []IntentTrace
	for rows.Next() {
		var t IntentTrace
		if err := rows.Scan(&t.ID, &t.OutcomeType, &t.OutcomeDetail, &t.TraceSummary,
			&t.ActivityWindowMinutes, &t.StartedAt, &t.CompletedAt, &t.RawEvents); err != nil {
			return nil, fmt.Errorf("scan trace row: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// GetTracesInRange returns intent traces whose completed_at falls within [start, end].
func GetTracesInRange(db *DB, start, end int64) ([]IntentTrace, error) {
	const q = `
SELECT id, outcome_type, COALESCE(outcome_detail,''), COALESCE(trace_summary,''),
       activity_window_minutes, started_at, completed_at, COALESCE(raw_events,'{}')
FROM intent_traces
WHERE completed_at >= ? AND completed_at <= ?
ORDER BY completed_at DESC`

	rows, err := db.Query(q, start, end)
	if err != nil {
		return nil, fmt.Errorf("query traces in range: %w", err)
	}
	defer rows.Close()

	var traces []IntentTrace
	for rows.Next() {
		var t IntentTrace
		if err := rows.Scan(&t.ID, &t.OutcomeType, &t.OutcomeDetail, &t.TraceSummary,
			&t.ActivityWindowMinutes, &t.StartedAt, &t.CompletedAt, &t.RawEvents); err != nil {
			return nil, fmt.Errorf("scan trace row: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// GetTracesWithEmbedding returns intent traces that have embeddings, within a time range.
func GetTracesWithEmbedding(db *DB, sinceUnix int64) ([]IntentTrace, error) {
	const q = `
SELECT id, outcome_type, COALESCE(outcome_detail,''), COALESCE(trace_summary,''),
       embedding, activity_window_minutes, started_at, completed_at
FROM intent_traces
WHERE completed_at >= ? AND embedding IS NOT NULL
ORDER BY completed_at DESC`

	rows, err := db.Query(q, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("query traces with embedding: %w", err)
	}
	defer rows.Close()

	var traces []IntentTrace
	for rows.Next() {
		var t IntentTrace
		if err := rows.Scan(&t.ID, &t.OutcomeType, &t.OutcomeDetail, &t.TraceSummary,
			&t.Embedding, &t.ActivityWindowMinutes, &t.StartedAt, &t.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan trace row: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// GetActivityInRange returns activity entries within a unix time range.
func GetActivityInRange(db *DB, start, end int64) ([]ActivityEntry, error) {
	const q = `
SELECT id, app_name, COALESCE(bundle_id,''), COALESCE(window_title,''),
       started_at, COALESCE(ended_at,0), COALESCE(duration_ms,0)
FROM activity_log
WHERE started_at >= ? AND started_at <= ?
ORDER BY started_at ASC`

	rows, err := db.Query(q, start, end)
	if err != nil {
		return nil, fmt.Errorf("query activity range: %w", err)
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.ID, &e.AppName, &e.BundleID, &e.WindowTitle,
			&e.StartedAt, &e.EndedAt, &e.DurationMs); err != nil {
			return nil, fmt.Errorf("scan activity row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetClipboardInRange returns clipboard items within a unix time range.
func GetClipboardInRange(db *DB, start, end int64) ([]ClipboardItem, error) {
	const q = `
SELECT id, content, content_type, COALESCE(source_app,''), COALESCE(bundle_id,''),
       COALESCE(window_title,''), captured_at
FROM clipboard_history
WHERE captured_at >= ? AND captured_at <= ?
ORDER BY captured_at ASC`

	rows, err := db.Query(q, start, end)
	if err != nil {
		return nil, fmt.Errorf("query clipboard range: %w", err)
	}
	defer rows.Close()

	var items []ClipboardItem
	for rows.Next() {
		var item ClipboardItem
		if err := rows.Scan(&item.ID, &item.Content, &item.ContentType,
			&item.SourceApp, &item.BundleID, &item.WindowTitle, &item.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan clipboard row: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetSessionsInRange returns context sessions within a unix time range.
func GetSessionsInRange(db *DB, start, end int64) ([]ContextSession, error) {
	const q = `
SELECT id, session_type, COALESCE(focus_score,0), started_at,
       COALESCE(ended_at,0), COALESCE(app_switches,0), COALESCE(dominant_app,'')
FROM context_sessions
WHERE started_at >= ? AND started_at <= ?
ORDER BY started_at ASC`

	rows, err := db.Query(q, start, end)
	if err != nil {
		return nil, fmt.Errorf("query sessions range: %w", err)
	}
	defer rows.Close()

	var sessions []ContextSession
	for rows.Next() {
		var s ContextSession
		if err := rows.Scan(&s.ID, &s.SessionType, &s.FocusScore,
			&s.StartedAt, &s.EndedAt, &s.AppSwitches, &s.DominantApp); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// GetRecentScreenshots returns the most recent n screenshots.
func GetRecentScreenshots(db *DB, n int) ([]Screenshot, error) {
	const q = `
SELECT id, file_path, COALESCE(source_app,''), COALESCE(bundle_id,''),
       COALESCE(window_title,''), captured_at
FROM screenshots
ORDER BY captured_at DESC
LIMIT ?`

	rows, err := db.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("query screenshots: %w", err)
	}
	defer rows.Close()

	var items []Screenshot
	for rows.Next() {
		var s Screenshot
		if err := rows.Scan(&s.ID, &s.FilePath, &s.SourceApp, &s.BundleID,
			&s.WindowTitle, &s.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan screenshot row: %w", err)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// GetVercelLogsInRange returns live:vercel log rows captured within a unix time range.
func GetVercelLogsInRange(db *DB, start, end int64) ([]VercelLog, error) {
	const q = `
SELECT id, deployment_url, COALESCE(level,''), COALESCE(method,''),
       COALESCE(path,''), COALESCE(status_code,0), message, captured_at
FROM live_vercel_logs
WHERE captured_at >= ? AND captured_at <= ?
ORDER BY captured_at ASC`

	rows, err := db.Query(q, start, end)
	if err != nil {
		return nil, fmt.Errorf("query vercel logs range: %w", err)
	}
	defer rows.Close()

	var items []VercelLog
	for rows.Next() {
		var v VercelLog
		if err := rows.Scan(&v.ID, &v.DeploymentURL, &v.Level, &v.Method,
			&v.Path, &v.StatusCode, &v.Message, &v.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan vercel log row: %w", err)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

// GetRecentVercelLogs returns the most recent n live:vercel log rows.
func GetRecentVercelLogs(db *DB, n int) ([]VercelLog, error) {
	const q = `
SELECT id, deployment_url, COALESCE(level,''), COALESCE(method,''),
       COALESCE(path,''), COALESCE(status_code,0), message, captured_at
FROM live_vercel_logs
ORDER BY captured_at DESC
LIMIT ?`

	rows, err := db.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("query recent vercel logs: %w", err)
	}
	defer rows.Close()

	var items []VercelLog
	for rows.Next() {
		var v VercelLog
		if err := rows.Scan(&v.ID, &v.DeploymentURL, &v.Level, &v.Method,
			&v.Path, &v.StatusCode, &v.Message, &v.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan vercel log row: %w", err)
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

// GetScreenshotsInRange returns screenshots within a unix time range.
func GetScreenshotsInRange(db *DB, start, end int64) ([]Screenshot, error) {
	const q = `
SELECT id, file_path, COALESCE(source_app,''), COALESCE(bundle_id,''),
       COALESCE(window_title,''), captured_at
FROM screenshots
WHERE captured_at >= ? AND captured_at <= ?
ORDER BY captured_at ASC`

	rows, err := db.Query(q, start, end)
	if err != nil {
		return nil, fmt.Errorf("query screenshots range: %w", err)
	}
	defer rows.Close()

	var items []Screenshot
	for rows.Next() {
		var s Screenshot
		if err := rows.Scan(&s.ID, &s.FilePath, &s.SourceApp, &s.BundleID,
			&s.WindowTitle, &s.CapturedAt); err != nil {
			return nil, fmt.Errorf("scan screenshot row: %w", err)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// GetExperienceCount returns the total number of experience records.
func GetExperienceCount(db *DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM experiences").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count experiences: %w", err)
	}
	return count, nil
}

// GetExperiencesWithEmbeddings returns all experiences that have embeddings.
func GetExperiencesWithEmbeddings(db *DB) ([]ExperienceRecord, error) {
	const q = `
SELECT id, task_intent, approach, COALESCE(tools_used,''), COALESCE(failure_points,''),
       COALESCE(resolution,''), outcome, COALESCE(tags,''), COALESCE(source,''),
       embedding, COALESCE(created_at,'')
FROM experiences
WHERE embedding IS NOT NULL`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("query experiences with embeddings: %w", err)
	}
	defer rows.Close()

	return scanExperienceRows(rows)
}

// SearchExperiencesByText performs a LIKE-based text search across experiences.
func SearchExperiencesByText(db *DB, query string, limit int) ([]ExperienceRecord, error) {
	const q = `
SELECT id, task_intent, approach, COALESCE(tools_used,''), COALESCE(failure_points,''),
       COALESCE(resolution,''), outcome, COALESCE(tags,''), COALESCE(source,''),
       embedding, COALESCE(created_at,'')
FROM experiences
WHERE task_intent LIKE ? OR approach LIKE ? OR tags LIKE ?
ORDER BY created_at DESC
LIMIT ?`

	pattern := "%" + query + "%"
	rows, err := db.Query(q, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search experiences: %w", err)
	}
	defer rows.Close()

	return scanExperienceRows(rows)
}

// GetRecentExperiences returns the most recent n experience records.
func GetRecentExperiences(db *DB, n int) ([]ExperienceRecord, error) {
	const q = `
SELECT id, task_intent, approach, COALESCE(tools_used,''), COALESCE(failure_points,''),
       COALESCE(resolution,''), outcome, COALESCE(tags,''), COALESCE(source,''),
       embedding, COALESCE(created_at,'')
FROM experiences
ORDER BY created_at DESC
LIMIT ?`

	rows, err := db.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("query recent experiences: %w", err)
	}
	defer rows.Close()

	return scanExperienceRows(rows)
}

// GetExperienceStats returns outcome counts.
func GetExperienceStats(db *DB) (total, success, failure, partial int, err error) {
	err = db.QueryRow("SELECT COUNT(*) FROM experiences").Scan(&total)
	if err != nil {
		return
	}
	db.QueryRow("SELECT COUNT(*) FROM experiences WHERE outcome='success'").Scan(&success)
	db.QueryRow("SELECT COUNT(*) FROM experiences WHERE outcome='failure'").Scan(&failure)
	db.QueryRow("SELECT COUNT(*) FROM experiences WHERE outcome='partial'").Scan(&partial)
	return
}

// GetProjectByID returns a v2 project by row ID.
func GetProjectByID(db *DB, id int64) (*Project, error) {
	const q = `
SELECT id, project_key, name, kind, root_hint, created_at, updated_at, metadata_json
FROM projects
WHERE id = ?`
	var p Project
	err := db.QueryRow(q, id).Scan(&p.ID, &p.ProjectKey, &p.Name, &p.Kind, &p.RootHint, &p.CreatedAt, &p.UpdatedAt, &p.MetadataJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}
	return &p, nil
}

// GetProjectByKey returns a v2 project by stable project key.
func GetProjectByKey(db *DB, key string) (*Project, error) {
	const q = `
SELECT id, project_key, name, kind, root_hint, created_at, updated_at, metadata_json
FROM projects
WHERE project_key = ?`
	var p Project
	err := db.QueryRow(q, key).Scan(&p.ID, &p.ProjectKey, &p.Name, &p.Kind, &p.RootHint, &p.CreatedAt, &p.UpdatedAt, &p.MetadataJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project by key: %w", err)
	}
	return &p, nil
}

// GetActiveOpenTask returns the most recently updated active or paused v2 task.
func GetActiveOpenTask(db *DB) (*OpenTask, error) {
	const q = `
SELECT id, task_id, COALESCE(project_id,0), status, goal, last_useful_state,
       next_action, confidence, started_at, updated_at, COALESCE(completed_at,0),
       completion_source, metadata_json
FROM open_tasks
WHERE status IN ('active', 'paused')
ORDER BY updated_at DESC
LIMIT 1`
	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("query active open task: %w", err)
	}
	defer rows.Close()
	tasks, err := scanOpenTasks(rows)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return &tasks[0], nil
}

// GetOpenTaskByTaskID returns a v2 task by stable task ID.
func GetOpenTaskByTaskID(db *DB, taskID string) (*OpenTask, error) {
	const q = `
SELECT id, task_id, COALESCE(project_id,0), status, goal, last_useful_state,
       next_action, confidence, started_at, updated_at, COALESCE(completed_at,0),
       completion_source, metadata_json
FROM open_tasks
WHERE task_id = ?`
	rows, err := db.Query(q, taskID)
	if err != nil {
		return nil, fmt.Errorf("query open task: %w", err)
	}
	defer rows.Close()
	tasks, err := scanOpenTasks(rows)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	return &tasks[0], nil
}

// GetRecentHumanTraces returns recent v2 traces.
func GetRecentHumanTraces(db *DB, n int) ([]HumanTrace, error) {
	const q = `
SELECT id, trace_id, trace_type, COALESCE(project_id,0), task_id, trigger_type,
       status, started_at, completed_at, summary, next_action, confidence, metadata_json
FROM human_traces
ORDER BY completed_at DESC
LIMIT ?`
	rows, err := db.Query(q, n)
	if err != nil {
		return nil, fmt.Errorf("query recent human traces: %w", err)
	}
	defer rows.Close()
	return scanHumanTraces(rows)
}

// GetHumanTraceByTraceID returns a v2 trace by stable trace ID.
func GetHumanTraceByTraceID(db *DB, traceID string) (*HumanTrace, error) {
	const q = `
SELECT id, trace_id, trace_type, COALESCE(project_id,0), task_id, trigger_type,
       status, started_at, completed_at, summary, next_action, confidence, metadata_json
FROM human_traces
WHERE trace_id = ?`
	rows, err := db.Query(q, traceID)
	if err != nil {
		return nil, fmt.Errorf("query human trace: %w", err)
	}
	defer rows.Close()
	traces, err := scanHumanTraces(rows)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, nil
	}
	return &traces[0], nil
}

// GetLatestHumanTraceByType returns the latest v2 trace of a type.
func GetLatestHumanTraceByType(db *DB, traceType string) (*HumanTrace, error) {
	const q = `
SELECT id, trace_id, trace_type, COALESCE(project_id,0), task_id, trigger_type,
       status, started_at, completed_at, summary, next_action, confidence, metadata_json
FROM human_traces
WHERE trace_type = ?
ORDER BY completed_at DESC
LIMIT 1`
	rows, err := db.Query(q, traceType)
	if err != nil {
		return nil, fmt.Errorf("query latest human trace: %w", err)
	}
	defer rows.Close()
	traces, err := scanHumanTraces(rows)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, nil
	}
	return &traces[0], nil
}

// GetHumanTraceEvents returns ordered v2 evidence events for a trace.
func GetHumanTraceEvents(db *DB, traceID string) ([]HumanTraceEvent, error) {
	const q = `
SELECT id, trace_id, ts, event_type, source_table, COALESCE(source_id,0),
       summary, app_name, window_title, artifact_uri, severity, metadata_json
FROM human_trace_events
WHERE trace_id = ?
ORDER BY ts ASC`
	rows, err := db.Query(q, traceID)
	if err != nil {
		return nil, fmt.Errorf("query human trace events: %w", err)
	}
	defer rows.Close()

	var events []HumanTraceEvent
	for rows.Next() {
		var e HumanTraceEvent
		if err := rows.Scan(&e.ID, &e.TraceID, &e.Ts, &e.EventType, &e.SourceTable,
			&e.SourceID, &e.Summary, &e.AppName, &e.WindowTitle, &e.ArtifactURI,
			&e.Severity, &e.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan human trace event row: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func scanOpenTasks(rows *sql.Rows) ([]OpenTask, error) {
	var tasks []OpenTask
	for rows.Next() {
		var t OpenTask
		if err := rows.Scan(&t.ID, &t.TaskID, &t.ProjectID, &t.Status, &t.Goal,
			&t.LastUsefulState, &t.NextAction, &t.Confidence, &t.StartedAt,
			&t.UpdatedAt, &t.CompletedAt, &t.CompletionSource, &t.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan open task row: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func scanHumanTraces(rows *sql.Rows) ([]HumanTrace, error) {
	var traces []HumanTrace
	for rows.Next() {
		var t HumanTrace
		if err := rows.Scan(&t.ID, &t.TraceID, &t.TraceType, &t.ProjectID, &t.TaskID,
			&t.TriggerType, &t.Status, &t.StartedAt, &t.CompletedAt, &t.Summary,
			&t.NextAction, &t.Confidence, &t.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan human trace row: %w", err)
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

func scanExperienceRows(rows *sql.Rows) ([]ExperienceRecord, error) {
	var records []ExperienceRecord
	for rows.Next() {
		var e ExperienceRecord
		var toolsStr, failStr, tagsStr string
		var embBlob []byte
		if err := rows.Scan(&e.ID, &e.TaskIntent, &e.Approach, &toolsStr, &failStr,
			&e.Resolution, &e.Outcome, &tagsStr, &e.Source, &embBlob, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan experience row: %w", err)
		}
		if toolsStr != "" {
			json.Unmarshal([]byte(toolsStr), &e.ToolsUsed)
		}
		if failStr != "" {
			json.Unmarshal([]byte(failStr), &e.FailurePoints)
		}
		if tagsStr != "" {
			json.Unmarshal([]byte(tagsStr), &e.Tags)
		}
		e.Embedding = embBlob
		records = append(records, e)
	}
	return records, rows.Err()
}

// SearchAll performs FTS search across clipboard content.
func SearchAll(db *DB, query string, limit int) ([]SearchResult, error) {
	const q = `
SELECT c.id, c.content, COALESCE(c.source_app,''), COALESCE(c.window_title,''),
       c.captured_at, rank
FROM clipboard_fts
JOIN clipboard_history c ON c.id = clipboard_fts.rowid
WHERE clipboard_fts MATCH ?
ORDER BY rank
LIMIT ?`

	rows, err := db.Query(q, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		r.Type = "clipboard"
		if err := rows.Scan(&r.ID, &r.Content, &r.SourceApp,
			&r.WindowTitle, &r.CapturedAt, &r.Rank); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
