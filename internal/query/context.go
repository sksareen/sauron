package query

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// LocalServer represents a process listening on localhost.
type LocalServer struct {
	Port    string `json:"port"`
	Process string `json:"process"`
	PID     string `json:"pid"`
}

// ContextSummary is a high-level view of what the user is doing right now.
type ContextSummary struct {
	// Semantic layer — what matters for re-entry
	OpenThread      string   `json:"open_thread,omitempty"`
	NextAction      string   `json:"next_action,omitempty"`
	RecentDecisions []string `json:"recent_decisions,omitempty"`
	// Mechanics — session state and environment
	SessionType     string        `json:"session_type"`
	FocusScore      float64       `json:"focus_score"`
	SessionAgeMin   float64       `json:"session_age_min"`
	DominantApp     string        `json:"dominant_app"`
	RecentClipboard []string      `json:"recent_clipboard"`
	LocalServers    []LocalServer `json:"local_servers,omitempty"`
}

// GetContext builds a ContextSummary from the database.
func GetContext(db *store.DB) (*ContextSummary, error) {
	session, err := store.GetCurrentSession(db)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	s := &ContextSummary{}

	// Pull open task first — the most important thing to surface on re-entry.
	if task, err := store.GetActiveOpenTask(db); err == nil && task != nil {
		s.OpenThread = task.Goal
		s.NextAction = task.NextAction
	}

	// Last 2 logged experiences as decision trail.
	if decisions, err := store.GetRecentExperiences(db, 2); err == nil {
		for _, d := range decisions {
			s.RecentDecisions = append(s.RecentDecisions, d.TaskIntent)
		}
	}

	if session != nil {
		s.SessionType = session.SessionType
		s.FocusScore = session.FocusScore
		s.DominantApp = session.DominantApp
		s.SessionAgeMin = float64(time.Now().Unix()-session.StartedAt) / 60.0
	} else {
		s.SessionType = "unknown"
		s.FocusScore = 0
	}

	// Last 3 clipboard items, truncated to 80 chars.
	items, err := store.GetRecentClipboard(db, 3)
	if err != nil {
		return nil, fmt.Errorf("get clipboard: %w", err)
	}
	for _, item := range items {
		snippet := item.Content
		if len(snippet) > 80 {
			snippet = snippet[:77] + "..."
		}
		s.RecentClipboard = append(s.RecentClipboard, snippet)
	}

	// Dominant app from last 2 hours if not set from session.
	if s.DominantApp == "" {
		entries, err := store.GetActivity(db, 2)
		if err == nil && len(entries) > 0 {
			s.DominantApp = dominantAppFrom(entries)
		}
	}

	// Listening localhost servers.
	s.LocalServers = getLocalServers()

	return s, nil
}

// FormatContext formats a ContextSummary in the requested format.
// format: "human" | "json" | "md"
func FormatContext(s *ContextSummary, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(s, "", "  ")
		return string(b)
	case "md":
		return formatContextMD(s)
	default:
		return formatContextHuman(s)
	}
}

func formatContextHuman(s *ContextSummary) string {
	var sb strings.Builder
	hasSemantics := s.OpenThread != "" || len(s.RecentDecisions) > 0
	if s.OpenThread != "" {
		sb.WriteString(fmt.Sprintf("open:       %s\n", s.OpenThread))
	}
	if s.NextAction != "" {
		sb.WriteString(fmt.Sprintf("next:       %s\n", s.NextAction))
	}
	for _, d := range s.RecentDecisions {
		sb.WriteString(fmt.Sprintf("decided:    %s\n", d))
	}
	if hasSemantics {
		sb.WriteString("---\n")
	}
	sb.WriteString(fmt.Sprintf("session:    %s\n", s.SessionType))
	sb.WriteString(fmt.Sprintf("focus:      %.0f%%\n", s.FocusScore*100))
	if s.SessionAgeMin > 0 {
		sb.WriteString(fmt.Sprintf("age:        %.0f min\n", s.SessionAgeMin))
	}
	if s.DominantApp != "" {
		sb.WriteString(fmt.Sprintf("app:        %s\n", s.DominantApp))
	}
	if len(s.LocalServers) > 0 {
		sb.WriteString("servers:\n")
		for _, srv := range s.LocalServers {
			sb.WriteString(fmt.Sprintf("  :%s  %s (pid %s)\n", srv.Port, srv.Process, srv.PID))
		}
	}
	if len(s.RecentClipboard) > 0 {
		sb.WriteString("clipboard:\n")
		for i, c := range s.RecentClipboard {
			sb.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, c))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatContextMD(s *ContextSummary) string {
	var sb strings.Builder
	sb.WriteString("## Current Context\n\n")
	sb.WriteString(fmt.Sprintf("- **Session**: %s (focus: %.0f%%)\n", s.SessionType, s.FocusScore*100))
	if s.SessionAgeMin > 0 {
		sb.WriteString(fmt.Sprintf("- **Time in session**: %.0f min\n", s.SessionAgeMin))
	}
	if s.DominantApp != "" {
		sb.WriteString(fmt.Sprintf("- **Active app**: %s\n", s.DominantApp))
	}
	if len(s.LocalServers) > 0 {
		sb.WriteString("\n### Local Servers\n\n")
		sb.WriteString("| Port | Process | PID |\n|------|---------|-----|\n")
		for _, srv := range s.LocalServers {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", srv.Port, srv.Process, srv.PID))
		}
	}
	if len(s.RecentClipboard) > 0 {
		sb.WriteString("\n### Recent Clipboard\n\n")
		for _, c := range s.RecentClipboard {
			sb.WriteString(fmt.Sprintf("- `%s`\n", strings.ReplaceAll(c, "`", "'")))
		}
	}
	return sb.String()
}

// getLocalServers uses lsof to find processes listening on localhost TCP ports.
func getLocalServers() []LocalServer {
	out, err := exec.Command("lsof", "+c", "0", "-iTCP", "-sTCP:LISTEN", "-nP").Output()
	if err != nil {
		return nil
	}

	// System processes to skip — not dev servers.
	skipProcs := map[string]bool{
		"ControlCenter": true, "rapportd": true, "Spotify": true,
		"Raycast": true, "sharingd": true, "AirPlayMDNSResponder": true,
	}

	seen := make(map[string]bool)
	var servers []LocalServer

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] == "COMMAND" {
			continue
		}
		cmd := fields[0]
		pid := fields[1]
		addr := fields[8]

		if skipProcs[cmd] {
			continue
		}

		// Extract port from address like *:3000 or 127.0.0.1:8080
		parts := strings.Split(addr, ":")
		if len(parts) < 2 {
			continue
		}
		port := parts[len(parts)-1]

		// Skip high ephemeral ports (49152+) — OS-assigned, not dev servers.
		portNum, err := strconv.Atoi(port)
		if err != nil || portNum >= 49152 {
			continue
		}

		key := port + "/" + pid
		if seen[key] {
			continue
		}
		seen[key] = true

		servers = append(servers, LocalServer{
			Port:    port,
			Process: cmd,
			PID:     pid,
		})
	}

	return servers
}

func dominantAppFrom(entries []store.ActivityEntry) string {
	counts := make(map[string]int64)
	for _, e := range entries {
		counts[e.AppName] += e.DurationMs
	}
	var best string
	var bestMs int64
	for app, ms := range counts {
		if ms > bestMs {
			best = app
			bestMs = ms
		}
	}
	return best
}
