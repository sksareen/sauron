package query

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

func TestValinorJSONContracts(t *testing.T) {
	if !strings.HasPrefix(strings.TrimSpace(FormatTimeline(nil, "json")), "[") {
		t.Fatal("timeline json must be an array")
	}
	if !strings.HasPrefix(strings.TrimSpace(FormatClipboard(nil, "json")), "[") {
		t.Fatal("clipboard json must be an array")
	}
	if !strings.HasPrefix(strings.TrimSpace(FormatRecentExperiences(nil, "json")), "[") {
		t.Fatal("recent experiences json must be an array")
	}
	if !strings.HasPrefix(strings.TrimSpace(FormatCheckExperience(nil, 0, "json")), "[") {
		t.Fatal("experience search json must be an array")
	}
	hints := FormatHints(nil, "json")
	var arr []any
	if err := json.Unmarshal([]byte(hints), &arr); err != nil {
		t.Fatal(err)
	}
}

func TestReentryAndHintRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	now := time.Unix(1_700_000_000, 0)
	if err := store.InsertSession(db, "deep_focus", 0.9, now.Add(-10*time.Minute).Unix(), 1, "Cursor"); err != nil {
		t.Fatal(err)
	}

	re, err := BuildReentry(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if re.Task == nil || re.Task.Status != "active" || re.Task.Goal != "Continue in Cursor" {
		t.Fatalf("reentry: %+v", re.Task)
	}
	if !ValinorTaskID(re.Task.TaskID) {
		t.Fatalf("task id %q is not a Valinor token", re.Task.TaskID)
	}
	raw := FormatReentry(re, "json")
	var parsed Reentry
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Project == nil || parsed.Project.Name != "Cursor" {
		t.Fatalf("project: %+v", parsed.Project)
	}

	if err := store.CompleteTask(db, re.Task.TaskID); err != nil {
		t.Fatal(err)
	}
	after, err := BuildReentry(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if after.Task != nil {
		t.Fatalf("completed task still active: %+v", after.Task)
	}

	id := "hint-1700000000000"
	if err := store.StartManualHint(db, id, "finish the letter"); err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListManualHints(db)
	if err != nil {
		t.Fatal(err)
	}
	body := FormatHints(listed, "json")
	var hints []store.ManualHint
	if err := json.Unmarshal([]byte(body), &hints); err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 || hints[0].Source != "manual" || hints[0].Status != "active" || hints[0].Label != "finish the letter" {
		t.Fatalf("hints: %+v", hints)
	}
	if err := store.StopManualHint(db, id); err != nil {
		t.Fatal(err)
	}
	listed, err = store.ListManualHints(db)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Status != "stopped" {
		t.Fatalf("status %q", listed[0].Status)
	}
}

func TestReentryPrefersRecentTrace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := store.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	now := time.Unix(1_700_000_000, 0)
	if err := store.InsertSession(db, "exploration", 0.4, now.Add(-30*time.Minute).Unix(), 4, "Terminal"); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertIntentTrace(db, &store.IntentTrace{
		OutcomeType:           "git_commit",
		OutcomeDetail:         "Fix login refresh",
		TraceSummary:          "updated token refresh",
		ActivityWindowMinutes: 30,
		StartedAt:             now.Add(-20 * time.Minute).Unix(),
		CompletedAt:           now.Add(-5 * time.Minute).Unix(),
		RawEvents:             "{}",
	}); err != nil {
		t.Fatal(err)
	}

	re, err := BuildReentry(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if re.Task == nil || re.Task.Goal != "Fix login refresh" || re.Project.Name != "git_commit" {
		t.Fatalf("%+v", re)
	}
	if !strings.HasPrefix(re.Task.TaskID, "trace-") {
		t.Fatal(re.Task.TaskID)
	}
}
