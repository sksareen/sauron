package scrub

import (
	"regexp"

	"github.com/sksareen/sauron/internal/store"
)

var patterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// API keys, tokens, secrets in assignments.
	{regexp.MustCompile(`(?i)(?:sk|pk|api[_-]?key|token|secret|password|auth)[_-]?\w*\s*[=:]\s*['"]?[\w\-./]{20,}['"]?`), "[REDACTED_KEY]"},
	// Bearer tokens.
	{regexp.MustCompile(`(?i)Bearer\s+[\w\-./]{20,}`), "[REDACTED_TOKEN]"},
	// Email addresses.
	{regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`), "[EMAIL]"},
	// IP addresses (skip localhost and 0.0.0.0).
	{regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\b`), "[IP_ADDR]"},
	// Home directory paths.
	{regexp.MustCompile(`/Users/\w+`), "~"},
	{regexp.MustCompile(`/home/\w+`), "~"},
	// AWS keys.
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[AWS_KEY]"},
	// Private key blocks.
	{regexp.MustCompile(`(?s)-----BEGIN [\w ]+ KEY-----.*?-----END [\w ]+ KEY-----`), "[PRIVATE_KEY]"},
	// Connection strings.
	{regexp.MustCompile(`(?i)(?:postgres|mysql|mongodb|redis)://[^\s"']+`), "[CONNECTION_STRING]"},
}

// Scrub redacts sensitive patterns from a string.
func Scrub(text string) string {
	for _, p := range patterns {
		text = p.re.ReplaceAllString(text, p.repl)
	}
	return text
}

// ScrubRecord scrubs all string fields in an ExperienceRecord.
func ScrubRecord(r *store.ExperienceRecord) {
	r.TaskIntent = Scrub(r.TaskIntent)
	r.Approach = Scrub(r.Approach)
	r.Resolution = Scrub(r.Resolution)
	for i, v := range r.ToolsUsed {
		r.ToolsUsed[i] = Scrub(v)
	}
	for i, v := range r.FailurePoints {
		r.FailurePoints[i] = Scrub(v)
	}
	for i, v := range r.Tags {
		r.Tags[i] = Scrub(v)
	}
}
