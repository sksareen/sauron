package store

import (
	"testing"
)

func TestVercelLogInsertAndDedupe(t *testing.T) {
	db := testDB(t)

	rec := &VercelLog{
		DeploymentURL: "https://example.vercel.app",
		Level:         "error",
		Method:        "POST",
		Path:          "/api/interview/start",
		StatusCode:    500,
		Message:       "TypeError: Cannot read properties of undefined",
		CapturedAt:    1713600000,
	}

	inserted, err := InsertVercelLog(db, rec, "hash-aaa")
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !inserted {
		t.Fatal("expected first insert to succeed")
	}

	// Same dedupe hash should be a no-op.
	inserted, err = InsertVercelLog(db, rec, "hash-aaa")
	if err != nil {
		t.Fatalf("duplicate insert: %v", err)
	}
	if inserted {
		t.Fatal("expected duplicate insert to be ignored")
	}

	// A different hash should insert.
	rec2 := *rec
	rec2.Message = "A different message"
	inserted, err = InsertVercelLog(db, &rec2, "hash-bbb")
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if !inserted {
		t.Fatal("expected second insert to succeed")
	}

	// Range query returns both rows.
	rows, err := GetVercelLogsInRange(db, 0, 9999999999)
	if err != nil {
		t.Fatalf("range query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Message == "" || rows[0].DeploymentURL != "https://example.vercel.app" {
		t.Fatalf("row 0 unexpected: %+v", rows[0])
	}

	// Recent query returns both rows (newest first order).
	recent, err := GetRecentVercelLogs(db, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent, got %d", len(recent))
	}
}

func TestVercelLogNullableFields(t *testing.T) {
	db := testDB(t)

	// Minimal record — no method/path/level/status.
	rec := &VercelLog{
		DeploymentURL: "https://example.vercel.app",
		Message:       "generic message with no request metadata",
		CapturedAt:    1713600001,
	}
	inserted, err := InsertVercelLog(db, rec, "hash-min")
	if err != nil {
		t.Fatalf("insert minimal: %v", err)
	}
	if !inserted {
		t.Fatal("expected insert to succeed")
	}

	rows, err := GetRecentVercelLogs(db, 1)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1, got %d", len(rows))
	}
	r := rows[0]
	if r.Level != "" || r.Method != "" || r.Path != "" || r.StatusCode != 0 {
		t.Fatalf("expected empty/zero metadata, got %+v", r)
	}
	if r.Message != "generic message with no request metadata" {
		t.Fatalf("wrong message: %q", r.Message)
	}
}
