package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/embed"
	"github.com/sksareen/sauron/internal/reentry"
	"github.com/sksareen/sauron/internal/store"
)

const (
	intentPollInterval = 10 * time.Second
	defaultTraceWindow = 30 // minutes
)

// intentPoller watches for outcomes (git commits, agentgraph entries) and creates intent traces.
type intentPoller struct {
	db *store.DB

	// Git tracking: map of repo dir -> last seen commit hash.
	gitRepos    map[string]string
	reposCached bool

	// AgentGraph tracking: last seen row ID.
	lastAgentGraphID int64
}

// runIntentPoller polls for outcomes every 10 seconds.
func runIntentPoller(ctx context.Context, db *store.DB) {
	p := &intentPoller{
		db:       db,
		gitRepos: make(map[string]string),
	}

	// Seed agentgraph last ID.
	p.lastAgentGraphID = p.getLastAgentGraphID()

	ticker := time.NewTicker(intentPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *intentPoller) poll() {
	if !p.reposCached {
		p.scanForRepos()
		p.reposCached = true
	}

	p.checkGitCommits()
	p.checkAgentGraph()
}

// scanForRepos finds git repos under ~/coding/*/.
func (p *intentPoller) scanForRepos() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	codingDir := filepath.Join(home, "coding")
	entries, err := os.ReadDir(codingDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoDir := filepath.Join(codingDir, e.Name())
		gitDir := filepath.Join(repoDir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			// Seed with current HEAD so we don't trigger on startup.
			hash := getGitHEAD(repoDir)
			p.gitRepos[repoDir] = hash
		}
	}

	log.Printf("intent: tracking %d git repos", len(p.gitRepos))
}

// checkGitCommits compares HEAD hashes for tracked repos.
func (p *intentPoller) checkGitCommits() {
	for dir, lastHash := range p.gitRepos {
		currentHash := getGitHEAD(dir)
		if currentHash == "" || currentHash == lastHash {
			continue
		}

		p.gitRepos[dir] = currentHash

		// Get commit message.
		msg := getGitCommitMessage(dir)
		detail := fmt.Sprintf("%s: %s", filepath.Base(dir), msg)

		log.Printf("intent: git commit detected in %s: %s", filepath.Base(dir), currentHash[:8])
		p.createTrace("git_commit", detail)
		if err := reentry.RecordCommitOutcome(p.db, dir, msg, currentHash, time.Now().Unix()); err != nil {
			log.Printf("reentry: git commit trace failed: %v", err)
		}
	}
}

// checkAgentGraph looks for new rows in the agentgraph experiences.db.
func (p *intentPoller) checkAgentGraph() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	dbPath := filepath.Join(home, ".agentgraph", "experiences.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return
	}

	agDB, err := sql.Open("sqlite", dbPath+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return
	}
	defer agDB.Close()

	var maxID int64
	err = agDB.QueryRow("SELECT COALESCE(MAX(id), 0) FROM experiences").Scan(&maxID)
	if err != nil {
		return
	}

	if maxID <= p.lastAgentGraphID {
		return
	}

	// Fetch new experiences.
	rows, err := agDB.Query(
		"SELECT id, COALESCE(description, '') FROM experiences WHERE id > ? ORDER BY id ASC",
		p.lastAgentGraphID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var desc string
		if err := rows.Scan(&id, &desc); err != nil {
			continue
		}

		detail := desc
		if len(detail) > 200 {
			detail = detail[:200]
		}

		log.Printf("intent: agentgraph experience detected: id=%d", id)
		p.createTrace("claude_session_end", detail)
		p.lastAgentGraphID = id
	}
}

// createTrace collects backward context and stores an intent trace.
func (p *intentPoller) createTrace(outcomeType, outcomeDetail string) {
	now := time.Now().Unix()
	windowStart := now - int64(defaultTraceWindow*60)

	// Collect raw events from the trace window.
	activities, _ := store.GetActivityInRange(p.db, windowStart, now)
	clipboards, _ := store.GetClipboardInRange(p.db, windowStart, now)
	sessions, _ := store.GetSessionsInRange(p.db, windowStart, now)

	// Build raw events JSON.
	rawEvents := map[string]interface{}{
		"activities": activities,
		"clipboard":  clipboards,
		"sessions":   sessions,
	}
	rawJSON, _ := json.Marshal(rawEvents)

	// Build a text summary for embedding.
	summary := buildTraceSummary(outcomeType, outcomeDetail, activities, clipboards, sessions)

	// Get embedding (best-effort).
	var embeddingBytes []byte
	vec, err := embed.GetEmbedding(summary)
	if err != nil {
		log.Printf("intent: embedding failed: %v", err)
	}
	if vec != nil {
		embeddingBytes = embed.VectorToBytes(vec)
	}

	trace := &store.IntentTrace{
		OutcomeType:           outcomeType,
		OutcomeDetail:         outcomeDetail,
		TraceSummary:          summary,
		Embedding:             embeddingBytes,
		ActivityWindowMinutes: defaultTraceWindow,
		StartedAt:             windowStart,
		CompletedAt:           now,
		RawEvents:             string(rawJSON),
	}

	if err := store.InsertIntentTrace(p.db, trace); err != nil {
		log.Printf("intent: failed to insert trace: %v", err)
	} else {
		log.Printf("intent: trace recorded (%s)", outcomeType)
	}
}

// buildTraceSummary creates a text summary from the trace data.
func buildTraceSummary(outcomeType, outcomeDetail string, activities []store.ActivityEntry, clipboards []store.ClipboardItem, sessions []store.ContextSession) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Outcome: %s — %s\n", outcomeType, outcomeDetail))

	// Session types.
	if len(sessions) > 0 {
		types := make(map[string]int)
		for _, s := range sessions {
			types[s.SessionType]++
		}
		sb.WriteString("Sessions: ")
		first := true
		for t, c := range types {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("%s(%d)", t, c))
			first = false
		}
		sb.WriteString("\n")
	}

	// App usage.
	if len(activities) > 0 {
		apps := make(map[string]struct{})
		for _, a := range activities {
			apps[a.AppName] = struct{}{}
		}
		sb.WriteString("Apps: ")
		first := true
		for app := range apps {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(app)
			first = false
		}
		sb.WriteString("\n")
	}

	// Clipboard snippets (last 5, truncated).
	if len(clipboards) > 0 {
		sb.WriteString("Clipboard:\n")
		start := 0
		if len(clipboards) > 5 {
			start = len(clipboards) - 5
		}
		for _, c := range clipboards[start:] {
			snippet := c.Content
			if len(snippet) > 100 {
				snippet = snippet[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("  - %s\n", snippet))
		}
	}

	return sb.String()
}

// getGitHEAD returns the current HEAD commit hash for a git repo.
func getGitHEAD(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "log", "-1", "--format=%H")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getGitCommitMessage returns the most recent commit message.
func getGitCommitMessage(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "log", "-1", "--format=%s")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getLastAgentGraphID returns the current max ID from the agentgraph experiences table.
func (p *intentPoller) getLastAgentGraphID() int64 {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0
	}

	dbPath := filepath.Join(home, ".agentgraph", "experiences.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0
	}

	agDB, err := sql.Open("sqlite", dbPath+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return 0
	}
	defer agDB.Close()

	var maxID int64
	agDB.QueryRow("SELECT COALESCE(MAX(id), 0) FROM experiences").Scan(&maxID)
	return maxID
}
