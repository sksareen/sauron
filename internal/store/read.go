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
