package scrub

import (
	"strings"
	"testing"

	"github.com/sksareen/sauron/internal/store"
)

func TestScrubAPIKeys(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"api_key = 'sk-or-v1-abc123def456ghi789'", "[REDACTED_KEY]"},
		{"token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc", "[REDACTED_KEY]"},
		{"SECRET_KEY=supersecretvalue12345678901234567890", "[REDACTED_KEY]"},
	}
	for _, c := range cases {
		got := Scrub(c.input)
		if got != c.want {
			t.Errorf("Scrub(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestScrubBearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	got := Scrub(input)
	if !strings.Contains(got, "[REDACTED_TOKEN]") {
		t.Errorf("expected [REDACTED_TOKEN] in %q", got)
	}
}

func TestScrubEmail(t *testing.T) {
	got := Scrub("contact user@example.com for help")
	if !strings.Contains(got, "[EMAIL]") {
		t.Errorf("expected [EMAIL] in %q", got)
	}
}

func TestScrubHomePaths(t *testing.T) {
	got := Scrub("file at /Users/testuser/coding/project/main.go")
	if strings.Contains(got, "/Users/savar") {
		t.Errorf("home path not scrubbed: %q", got)
	}
	if !strings.Contains(got, "~/coding/project/main.go") {
		t.Errorf("expected ~ replacement in %q", got)
	}
}

func TestScrubAWSKey(t *testing.T) {
	got := Scrub("key: AKIAIOSFODNN7EXAMPLE")
	if !strings.Contains(got, "[AWS_KEY]") {
		t.Errorf("expected [AWS_KEY] in %q", got)
	}
}

func TestScrubConnectionString(t *testing.T) {
	got := Scrub("db: postgres://user:pass@host:5432/dbname")
	if !strings.Contains(got, "[CONNECTION_STRING]") {
		t.Errorf("expected [CONNECTION_STRING] in %q", got)
	}
}

func TestScrubPreservesNormalText(t *testing.T) {
	input := "Fixed the authentication bug in the login flow using React and TypeScript"
	got := Scrub(input)
	if got != input {
		t.Errorf("normal text was modified: %q -> %q", input, got)
	}
}

func TestScrubRecord(t *testing.T) {
	rec := &store.ExperienceRecord{
		TaskIntent:    "Deploy to /Users/testuser/coding/project",
		Approach:      "Used api_key = 'sk-or-v1-abc123def456ghi789' to authenticate",
		ToolsUsed:     []string{"go", "docker"},
		FailurePoints: []string{"user@example.com bounced"},
		Resolution:    "Fixed the postgres://user:pass@host/db connection",
	}
	ScrubRecord(rec)

	if strings.Contains(rec.TaskIntent, "/Users/savar") {
		t.Error("TaskIntent not scrubbed")
	}
	if !strings.Contains(rec.Approach, "[REDACTED_KEY]") {
		t.Error("Approach key not scrubbed")
	}
	if !strings.Contains(rec.FailurePoints[0], "[EMAIL]") {
		t.Error("FailurePoints email not scrubbed")
	}
	if !strings.Contains(rec.Resolution, "[CONNECTION_STRING]") {
		t.Error("Resolution connection string not scrubbed")
	}
}
