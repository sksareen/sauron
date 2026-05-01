package daemon

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// DefaultVercelDeploymentURL is the deployment the live:vercel source tails
// when SAURON_VERCEL_DEPLOYMENT_URL isn't set.
//
// This lives as a peer of clipboard/activity capture — any of the daemon's
// polling goroutines can use it as its "home" deployment.
const DefaultVercelDeploymentURL = "https://pm-interview-v2-izli6ymg3-sksareens-projects.vercel.app"

// vercelLogFilter matches log messages worth keeping in Sauron's timeline.
// Mirrors the filter Savar was using in his ad-hoc live-Claude monitor —
// API routes, errors, media work, and model calls. Everything else is too noisy.
var vercelLogFilter = regexp.MustCompile(
	`api/|ERROR|Error|error|transcribe|Transcribe|evaluate|chat|500|502|failed|timeout|gemini|gpt-|webm|wav|mp3`,
)

// maxVercelSummaryLen caps the `message` text we store so timeline rows stay
// roughly comparable in size to clipboard snippets.
const maxVercelSummaryLen = 280

// vercelLogEntry is the subset of `vercel logs --format=json` fields we parse.
// Vercel's JSON is richer than this — extra fields are simply dropped.
type vercelLogEntry struct {
	TimestampInMs int64  `json:"timestampInMs"`
	Level         string `json:"level"`
	Message       string `json:"message"`
	Source        string `json:"source"`
	RequestMethod string `json:"requestMethod"`
	RequestPath   string `json:"requestPath"`
	StatusCode    int    `json:"statusCode"`
}

// RunVercelPoller tails `vercel logs <url> --format=json` forever.
//
// The command self-terminates after ~5 min, so we loop it with a short backoff.
// Transient auth/network errors are logged, not fatal — this is a passive source.
//
// Exported so an out-of-tree smoke-test binary can run just this poller
// against a temp DB without starting the full daemon.
func RunVercelPoller(ctx context.Context, db *store.DB, deploymentURL string) {
	if deploymentURL == "" {
		log.Println("vercel: no deployment URL configured, source disabled")
		return
	}
	log.Printf("vercel: watching %s", deploymentURL)

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := streamVercelLogs(ctx, db, deploymentURL); err != nil {
			log.Printf("vercel: stream ended: %v", err)
		}

		// Backoff between restarts (1s, capped at 30s on repeat failures).
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

// streamVercelLogs runs a single `vercel logs` invocation and blocks until
// it exits (vercel self-terminates after ~5 min) or ctx is cancelled.
func streamVercelLogs(ctx context.Context, db *store.DB, deploymentURL string) error {
	cmd := exec.CommandContext(ctx, "vercel", "logs", deploymentURL, "--format=json")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start vercel cli: %w", err)
	}

	// Drain stderr into the daemon log so auth / network errors surface loudly
	// but don't crash the process.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) != "" {
				log.Printf("vercel[stderr]: %s", line)
			}
		}
	}()

	scanner := bufio.NewScanner(stdout)
	// Runtime log lines can be long (stack traces). Bump the buffer to 1MB.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			// Vercel CLI prints some human-readable preamble before JSON. Skip.
			continue
		}
		handleVercelLine(db, deploymentURL, line)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("vercel: scanner: %v", err)
	}

	// Release resources. Ignore exit code — the CLI exits non-zero on Ctrl-C.
	_ = cmd.Wait()
	return nil
}

// handleVercelLine parses one JSON log line, filters it, and inserts it.
// Bad JSON and uninteresting lines are silently dropped so an occasional
// garbled line doesn't spam the daemon log.
func handleVercelLine(db *store.DB, deploymentURL, line string) {
	var entry vercelLogEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return
	}

	if !keepVercelLog(&entry) {
		return
	}

	summary := trimSummary(entry.Message, maxVercelSummaryLen)
	if summary == "" {
		return
	}

	capturedAt := entry.TimestampInMs / 1000
	if capturedAt == 0 {
		capturedAt = time.Now().Unix()
	}

	dedupe := vercelDedupeHash(entry.TimestampInMs, entry.Message)

	rec := &store.VercelLog{
		DeploymentURL: deploymentURL,
		Level:         entry.Level,
		Method:        entry.RequestMethod,
		Path:          entry.RequestPath,
		StatusCode:    entry.StatusCode,
		Message:       summary,
		CapturedAt:    capturedAt,
	}

	inserted, err := store.InsertVercelLog(db, rec, dedupe)
	if err != nil {
		log.Printf("vercel: insert failed: %v", err)
		return
	}
	if inserted {
		log.Printf("vercel: %s %s %s %d", entry.Level, entry.RequestMethod, entry.RequestPath, entry.StatusCode)
	}
}

// keepVercelLog is the raw-only filter — no LLM, no semantic dedupe.
// Keep if level is error/warning, or message matches the interesting-pattern regex.
func keepVercelLog(e *vercelLogEntry) bool {
	lvl := strings.ToLower(e.Level)
	if lvl == "error" || lvl == "warning" || lvl == "warn" {
		return true
	}
	if e.Message == "" {
		return false
	}
	return vercelLogFilter.MatchString(e.Message)
}

// trimSummary normalizes whitespace and caps the message at n characters.
func trimSummary(msg string, n int) string {
	msg = strings.TrimSpace(msg)
	// Collapse internal newlines so timeline rows stay single-line.
	msg = strings.ReplaceAll(msg, "\r\n", " ")
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > n {
		msg = msg[:n-3] + "..."
	}
	return msg
}

// vercelDedupeHash is a stable key on (timestampInMs, message). A watcher
// crash-and-restart that replays overlapping lines won't double-insert.
func vercelDedupeHash(tsMs int64, msg string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d\x00%s", tsMs, msg)
	return hex.EncodeToString(h.Sum(nil))
}

// VercelDeploymentURL returns the deployment URL to watch, pulling from
// SAURON_VERCEL_DEPLOYMENT_URL if set, otherwise the default.
//
// Kept as an exported helper so the CLI can print it for `sauron status`.
func VercelDeploymentURL() string {
	if v := strings.TrimSpace(os.Getenv("SAURON_VERCEL_DEPLOYMENT_URL")); v != "" {
		return v
	}
	return DefaultVercelDeploymentURL
}

// EnsureVercelCLI verifies `vercel` is on PATH and authenticated. If not,
// it returns an error — the daemon logs it but keeps running other sources.
func EnsureVercelCLI(ctx context.Context) error {
	if _, err := exec.LookPath("vercel"); err != nil {
		return fmt.Errorf("vercel CLI not found on PATH: %w", err)
	}
	// `vercel whoami` exits non-zero when not logged in.
	cmd := exec.CommandContext(ctx, "vercel", "whoami")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vercel not authed (run `vercel login`): %s", strings.TrimSpace(string(out)))
	}
	return nil
}
