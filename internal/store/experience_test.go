package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	sqldb, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := sqldb.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	db := &DB{sqldb}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestExperienceCRUD(t *testing.T) {
	db := testDB(t)

	// Initially empty.
	count, err := GetExperienceCount(db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	// Insert.
	rec := &ExperienceRecord{
		TaskIntent: "Fix authentication bug in login flow",
		Approach:   "Traced the issue to expired JWT validation",
		Outcome:    "success",
		ToolsUsed:  []string{"go", "jwt-go"},
		Tags:       []string{"auth", "bugfix"},
		Source:     "test",
	}
	id, err := InsertExperience(db, rec)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}

	// Count.
	count, err = GetExperienceCount(db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	// Insert another.
	rec2 := &ExperienceRecord{
		TaskIntent:    "Deploy service to production",
		Approach:      "Used Docker with multi-stage builds",
		Outcome:       "failure",
		FailurePoints: []string{"OOM on build step"},
		Resolution:    "Increased memory limit",
		Tags:          []string{"deploy", "docker"},
		Source:        "test",
	}
	_, err = InsertExperience(db, rec2)
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	// Stats.
	total, success, failure, partial, err := GetExperienceStats(db)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if total != 2 || success != 1 || failure != 1 || partial != 0 {
		t.Fatalf("stats: total=%d success=%d failure=%d partial=%d", total, success, failure, partial)
	}

	// Text search.
	results, err := SearchExperiencesByText(db, "authentication", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].TaskIntent != "Fix authentication bug in login flow" {
		t.Fatalf("wrong result: %s", results[0].TaskIntent)
	}

	// Recent.
	recent, err := GetRecentExperiences(db, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2, got %d", len(recent))
	}

	// Verify JSON deserialization.
	for _, r := range recent {
		if r.TaskIntent == "Fix authentication bug in login flow" {
			if len(r.ToolsUsed) != 2 || r.ToolsUsed[0] != "go" {
				t.Fatalf("tools not deserialized: %v", r.ToolsUsed)
			}
			if len(r.Tags) != 2 || r.Tags[0] != "auth" {
				t.Fatalf("tags not deserialized: %v", r.Tags)
			}
		}
		if r.TaskIntent == "Deploy service to production" {
			if len(r.FailurePoints) != 1 || r.FailurePoints[0] != "OOM on build step" {
				t.Fatalf("failure_points not deserialized: %v", r.FailurePoints)
			}
			if r.Resolution != "Increased memory limit" {
				t.Fatalf("resolution wrong: %s", r.Resolution)
			}
		}
	}
}

func TestExperienceWithEmbedding(t *testing.T) {
	db := testDB(t)

	// With embedding.
	fakeEmb := make([]byte, 16) // 4 float32s
	rec := &ExperienceRecord{
		TaskIntent: "Test embedding storage",
		Approach:   "Store and retrieve vectors",
		Outcome:    "success",
		Embedding:  fakeEmb,
	}
	_, err := InsertExperience(db, rec)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Without embedding.
	rec2 := &ExperienceRecord{
		TaskIntent: "No embedding",
		Approach:   "Should not appear",
		Outcome:    "success",
	}
	_, err = InsertExperience(db, rec2)
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	// Only records with embeddings.
	withEmb, err := GetExperiencesWithEmbeddings(db)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(withEmb) != 1 {
		t.Fatalf("expected 1, got %d", len(withEmb))
	}
	if withEmb[0].TaskIntent != "Test embedding storage" {
		t.Fatalf("wrong: %s", withEmb[0].TaskIntent)
	}
	if len(withEmb[0].Embedding) != 16 {
		t.Fatalf("embedding size: %d", len(withEmb[0].Embedding))
	}
}

func TestExperienceOutcomeValidation(t *testing.T) {
	db := testDB(t)

	// Invalid outcome should fail.
	_, err := InsertExperience(db, &ExperienceRecord{
		TaskIntent: "Bad", Approach: "Bad", Outcome: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid outcome")
	}

	// Valid outcomes.
	for _, o := range []string{"success", "failure", "partial"} {
		_, err := InsertExperience(db, &ExperienceRecord{
			TaskIntent: "Valid " + o, Approach: "Test", Outcome: o,
		})
		if err != nil {
			t.Fatalf("valid outcome %q failed: %v", o, err)
		}
	}
}

func TestExperienceNullFields(t *testing.T) {
	db := testDB(t)

	// Minimal record — only required fields.
	_, err := InsertExperience(db, &ExperienceRecord{
		TaskIntent: "Minimal", Approach: "Just required fields", Outcome: "success",
	})
	if err != nil {
		t.Fatalf("insert minimal: %v", err)
	}

	recent, err := GetRecentExperiences(db, 1)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1, got %d", len(recent))
	}
	r := recent[0]
	if r.TaskIntent != "Minimal" {
		t.Fatalf("wrong: %s", r.TaskIntent)
	}
	if len(r.ToolsUsed) != 0 || len(r.Tags) != 0 || len(r.FailurePoints) != 0 {
		t.Fatal("expected empty slices for null JSON fields")
	}
	if r.Resolution != "" {
		t.Fatalf("expected empty resolution, got %q", r.Resolution)
	}
}
