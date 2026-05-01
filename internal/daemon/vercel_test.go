package daemon

import (
	"strings"
	"testing"
)

func TestKeepVercelLog(t *testing.T) {
	cases := []struct {
		name  string
		entry vercelLogEntry
		keep  bool
	}{
		{
			name:  "error level always kept",
			entry: vercelLogEntry{Level: "error", Message: "anything"},
			keep:  true,
		},
		{
			name:  "warning level kept",
			entry: vercelLogEntry{Level: "warning", Message: "anything"},
			keep:  true,
		},
		{
			name:  "warn alias kept",
			entry: vercelLogEntry{Level: "warn", Message: "anything"},
			keep:  true,
		},
		{
			name:  "info with api/ path kept",
			entry: vercelLogEntry{Level: "info", Message: "POST /api/interview/start 200"},
			keep:  true,
		},
		{
			name:  "gemini mention kept",
			entry: vercelLogEntry{Level: "info", Message: "calling gemini-2.0-flash"},
			keep:  true,
		},
		{
			name:  "gpt- mention kept",
			entry: vercelLogEntry{Level: "info", Message: "routed to gpt-4o"},
			keep:  true,
		},
		{
			name:  "status 500 kept",
			entry: vercelLogEntry{Level: "info", Message: "request finished 500"},
			keep:  true,
		},
		{
			name:  "generic boot log dropped",
			entry: vercelLogEntry{Level: "info", Message: "Ready in 128ms"},
			keep:  false,
		},
		{
			name:  "empty message with info level dropped",
			entry: vercelLogEntry{Level: "info", Message: ""},
			keep:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := keepVercelLog(&c.entry)
			if got != c.keep {
				t.Fatalf("keep=%v want %v for %+v", got, c.keep, c.entry)
			}
		})
	}
}

func TestTrimSummary(t *testing.T) {
	// Whitespace stripped, newlines collapsed.
	got := trimSummary("  line1\nline2\r\nline3  ", 100)
	if got != "line1 line2 line3" {
		t.Fatalf("got %q", got)
	}

	// Cap works and adds ellipsis.
	long := strings.Repeat("a", 400)
	got = trimSummary(long, 280)
	if len(got) != 280 {
		t.Fatalf("expected len 280, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ... suffix, got %q", got[len(got)-5:])
	}
}

func TestVercelDedupeHashStable(t *testing.T) {
	h1 := vercelDedupeHash(1713600000123, "hello")
	h2 := vercelDedupeHash(1713600000123, "hello")
	if h1 != h2 {
		t.Fatal("hash not stable for identical inputs")
	}
	h3 := vercelDedupeHash(1713600000124, "hello")
	if h1 == h3 {
		t.Fatal("hash collided across different timestamps")
	}
	h4 := vercelDedupeHash(1713600000123, "hello2")
	if h1 == h4 {
		t.Fatal("hash collided across different messages")
	}
}
