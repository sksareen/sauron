package store

import (
	"database/sql"
	"fmt"
)

// GetActiveHints returns hints with status active or paused, ordered by weight desc.
func GetActiveHints(db *DB, limit int) ([]HintRecord, error) {
	rows, err := db.Query(`
SELECT id, label, confidence, weight, status, dominant_app, window_pattern,
       COALESCE(merge_group_id,''), embedding, started_at, last_active_at, labelled_at, evidence_count
FROM hints
WHERE status IN ('active','paused')
ORDER BY weight DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("get active hints: %w", err)
	}
	defer rows.Close()
	return scanHints(rows)
}

// GetHintByID returns a single hint by ID.
func GetHintByID(db *DB, id string) (*HintRecord, error) {
	rows, err := db.Query(`
SELECT id, label, confidence, weight, status, dominant_app, window_pattern,
       COALESCE(merge_group_id,''), embedding, started_at, last_active_at, labelled_at, evidence_count
FROM hints WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hints, err := scanHints(rows)
	if err != nil || len(hints) == 0 {
		return nil, err
	}
	return &hints[0], nil
}

// FindMergeCandidate returns the best active hint to merge a new signal into,
// given the app name and window pattern. Returns nil if none qualifies.
func FindMergeCandidate(db *DB, app, windowPattern string, sinceTs int64) (*HintRecord, error) {
	rows, err := db.Query(`
SELECT id, label, confidence, weight, status, dominant_app, window_pattern,
       COALESCE(merge_group_id,''), embedding, started_at, last_active_at, labelled_at, evidence_count
FROM hints
WHERE status IN ('active','paused')
  AND dominant_app = ?
  AND last_active_at >= ?
ORDER BY last_active_at DESC
LIMIT 1`, app, sinceTs)
	if err != nil {
		return nil, fmt.Errorf("find merge candidate: %w", err)
	}
	defer rows.Close()
	hints, err := scanHints(rows)
	if err != nil || len(hints) == 0 {
		return nil, err
	}
	return &hints[0], nil
}

// GetHintsNeedingLabel returns active hints with enough new evidence to justify a label call.
func GetHintsNeedingLabel(db *DB, minNewEvidence int, minAgeWithoutLabel int64, now int64) ([]HintRecord, error) {
	rows, err := db.Query(`
SELECT h.id, h.label, h.confidence, h.weight, h.status, h.dominant_app, h.window_pattern,
       COALESCE(h.merge_group_id,''), h.embedding, h.started_at, h.last_active_at, h.labelled_at, h.evidence_count,
       COUNT(e.id) AS new_evidence
FROM hints h
LEFT JOIN hint_evidence e ON e.hint_id = h.id AND e.ts > h.labelled_at
WHERE h.status = 'active'
  AND h.weight > 0.3
GROUP BY h.id
HAVING (new_evidence >= ? OR (h.label = '' AND h.evidence_count >= 3))
   AND (h.labelled_at = 0 OR h.last_active_at > h.labelled_at + ?)
ORDER BY h.weight DESC
LIMIT 5`, minNewEvidence, minAgeWithoutLabel)
	if err != nil {
		return nil, fmt.Errorf("get hints needing label: %w", err)
	}
	defer rows.Close()

	var out []HintRecord
	for rows.Next() {
		var h HintRecord
		var newEvidence int // extra column not in HintRecord
		if err := rows.Scan(&h.ID, &h.Label, &h.Confidence, &h.Weight, &h.Status,
			&h.DominantApp, &h.WindowPattern, &h.MergeGroupID, &h.Embedding,
			&h.StartedAt, &h.LastActiveAt, &h.LabelledAt, &h.EvidenceCount, &newEvidence); err != nil {
			return nil, fmt.Errorf("scan hint row: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetHintEvidence returns the most recent evidence rows for a hint.
func GetHintEvidence(db *DB, hintID string, limit int) ([]HintEvidence, error) {
	rows, err := db.Query(`
SELECT id, hint_id, source_table, source_id, ts, summary, app_name, window_title, severity
FROM hint_evidence
WHERE hint_id=?
ORDER BY ts DESC
LIMIT ?`, hintID, limit)
	if err != nil {
		return nil, fmt.Errorf("get hint evidence: %w", err)
	}
	defer rows.Close()

	var out []HintEvidence
	for rows.Next() {
		var e HintEvidence
		if err := rows.Scan(&e.ID, &e.HintID, &e.SourceTable, &e.SourceID,
			&e.Ts, &e.Summary, &e.AppName, &e.WindowTitle, &e.Severity); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetRecentHints returns all hints (any status) ordered by last_active_at desc.
func GetRecentHints(db *DB, limit int) ([]HintRecord, error) {
	rows, err := db.Query(`
SELECT id, label, confidence, weight, status, dominant_app, window_pattern,
       COALESCE(merge_group_id,''), embedding, started_at, last_active_at, labelled_at, evidence_count
FROM hints
ORDER BY last_active_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent hints: %w", err)
	}
	defer rows.Close()
	return scanHints(rows)
}

// GetAllActiveHintsForDecay returns all hints that need weight decay applied.
func GetAllActiveHintsForDecay(db *DB) ([]HintRecord, error) {
	rows, err := db.Query(`
SELECT id, label, confidence, weight, status, dominant_app, window_pattern,
       COALESCE(merge_group_id,''), embedding, started_at, last_active_at, labelled_at, evidence_count
FROM hints
WHERE status IN ('active','paused') AND weight > 0`)
	if err != nil {
		return nil, fmt.Errorf("get hints for decay: %w", err)
	}
	defer rows.Close()
	return scanHints(rows)
}

func scanHints(rows *sql.Rows) ([]HintRecord, error) {
	var out []HintRecord
	for rows.Next() {
		var h HintRecord
		if err := rows.Scan(&h.ID, &h.Label, &h.Confidence, &h.Weight, &h.Status,
			&h.DominantApp, &h.WindowPattern, &h.MergeGroupID, &h.Embedding,
			&h.StartedAt, &h.LastActiveAt, &h.LabelledAt, &h.EvidenceCount); err != nil {
			return nil, fmt.Errorf("scan hint row: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
