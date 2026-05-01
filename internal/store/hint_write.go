package store

import "fmt"

// UpsertHint inserts or replaces a hint record.
func UpsertHint(db *DB, h *HintRecord) error {
	_, err := db.Exec(`
INSERT INTO hints (id, label, confidence, weight, status, dominant_app, window_pattern,
                  merge_group_id, embedding, started_at, last_active_at, labelled_at, evidence_count)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
    label          = excluded.label,
    confidence     = excluded.confidence,
    weight         = excluded.weight,
    status         = excluded.status,
    dominant_app   = excluded.dominant_app,
    window_pattern = excluded.window_pattern,
    merge_group_id = excluded.merge_group_id,
    embedding      = excluded.embedding,
    last_active_at = excluded.last_active_at,
    labelled_at    = excluded.labelled_at,
    evidence_count = excluded.evidence_count`,
		h.ID, h.Label, h.Confidence, h.Weight, h.Status,
		h.DominantApp, h.WindowPattern, nilStr(h.MergeGroupID), h.Embedding,
		h.StartedAt, h.LastActiveAt, h.LabelledAt, h.EvidenceCount,
	)
	return err
}

// AppendHintEvidence adds a single evidence row to a hint.
func AppendHintEvidence(db *DB, e *HintEvidence) error {
	_, err := db.Exec(`
INSERT INTO hint_evidence (hint_id, source_table, source_id, ts, summary, app_name, window_title, severity)
VALUES (?,?,?,?,?,?,?,?)`,
		e.HintID, e.SourceTable, e.SourceID, e.Ts, e.Summary, e.AppName, e.WindowTitle, e.Severity,
	)
	if err != nil {
		return fmt.Errorf("append hint evidence: %w", err)
	}
	_, err = db.Exec(`UPDATE hints SET evidence_count = evidence_count + 1, last_active_at = ? WHERE id = ?`, e.Ts, e.HintID)
	return err
}

// UpdateHintLabel writes an LLM-generated label back to a hint.
func UpdateHintLabel(db *DB, hintID, label string, confidence float64, labelledAt int64) error {
	_, err := db.Exec(
		`UPDATE hints SET label=?, confidence=?, labelled_at=? WHERE id=?`,
		label, confidence, labelledAt, hintID,
	)
	return err
}

// UpdateHintWeight updates the weight and status of a hint.
func UpdateHintWeight(db *DB, hintID string, weight float64, status string) error {
	_, err := db.Exec(
		`UPDATE hints SET weight=?, status=? WHERE id=?`,
		weight, status, hintID,
	)
	return err
}

func nilStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
