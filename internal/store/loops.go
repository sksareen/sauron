package store

import (
	"fmt"
	"time"
)

// ManualHint is a user-started thread Valinor can show and stop.
type ManualHint struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Label     string `json:"label"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at"`
}

// TaskCompleted reports whether Valinor already marked this inferred task done.
func TaskCompleted(db *DB, taskID string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM completed_tasks WHERE task_id = ?`, taskID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("completed task lookup: %w", err)
	}
	return n > 0, nil
}

// CompleteTask records that an inferred open loop was marked done.
func CompleteTask(db *DB, taskID string) error {
	_, err := db.Exec(
		`INSERT INTO completed_tasks (task_id, completed_at) VALUES (?, ?)
		 ON CONFLICT(task_id) DO UPDATE SET completed_at = excluded.completed_at`,
		taskID, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	return nil
}

// ListManualHints returns hints, optionally including stopped ones.
func ListManualHints(db *DB) ([]ManualHint, error) {
	rows, err := db.Query(`
SELECT id, source, label, status, updated_at
FROM manual_hints
ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list hints: %w", err)
	}
	defer rows.Close()

	hints := []ManualHint{}
	for rows.Next() {
		var h ManualHint
		if err := rows.Scan(&h.ID, &h.Source, &h.Label, &h.Status, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan hint: %w", err)
		}
		hints = append(hints, h)
	}
	return hints, rows.Err()
}

// StartManualHint records a user-started thread.
func StartManualHint(db *DB, id, label string) error {
	now := time.Now().Unix()
	_, err := db.Exec(`
INSERT INTO manual_hints (id, source, label, status, updated_at)
VALUES (?, 'manual', ?, 'active', ?)
ON CONFLICT(id) DO UPDATE SET
    label = excluded.label,
    source = 'manual',
    status = 'active',
    updated_at = excluded.updated_at`,
		id, label, now,
	)
	if err != nil {
		return fmt.Errorf("start hint: %w", err)
	}
	return nil
}

// StopManualHint marks a hint stopped. Unknown ids are recorded as stopped
// so Valinor's stop button is idempotent.
func StopManualHint(db *DB, id string) error {
	now := time.Now().Unix()
	res, err := db.Exec(
		`UPDATE manual_hints SET status = 'stopped', updated_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("stop hint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("stop hint rows: %w", err)
	}
	if n > 0 {
		return nil
	}
	_, err = db.Exec(`
INSERT INTO manual_hints (id, source, label, status, updated_at)
VALUES (?, 'manual', '', 'stopped', ?)`,
		id, now,
	)
	if err != nil {
		return fmt.Errorf("stop hint insert: %w", err)
	}
	return nil
}
