package query

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// TimelineEntry is a single event in a fused timeline.
type TimelineEntry struct {
	Timestamp int64  `json:"timestamp"`
	Type      string `json:"type"` // "activity", "clipboard", "session", "trace"
	Summary   string `json:"summary"`
}

// GetTimeline returns a fused timeline of all event types in a time range.
func GetTimeline(db *store.DB, startUnix, endUnix int64) ([]TimelineEntry, error) {
	var entries []TimelineEntry

	// Activity entries.
	activities, err := store.GetActivityInRange(db, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("timeline activities: %w", err)
	}
	for _, a := range activities {
		title := a.WindowTitle
		if title == "" {
			title = a.AppName
		}
		dur := ""
		if a.DurationMs > 0 {
			dur = fmt.Sprintf(" (%ds)", a.DurationMs/1000)
		}
		entries = append(entries, TimelineEntry{
			Timestamp: a.StartedAt,
			Type:      "activity",
			Summary:   fmt.Sprintf("%s — %s%s", a.AppName, title, dur),
		})
	}

	// Clipboard items.
	clipboards, err := store.GetClipboardInRange(db, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("timeline clipboard: %w", err)
	}
	for _, c := range clipboards {
		snippet := c.Content
		if len(snippet) > 80 {
			snippet = snippet[:77] + "..."
		}
		app := c.SourceApp
		if app == "" {
			app = "unknown"
		}
		entries = append(entries, TimelineEntry{
			Timestamp: c.CapturedAt,
			Type:      "clipboard",
			Summary:   fmt.Sprintf("[%s] %s", app, snippet),
		})
	}

	// Sessions.
	sessions, err := store.GetSessionsInRange(db, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("timeline sessions: %w", err)
	}
	for _, s := range sessions {
		entries = append(entries, TimelineEntry{
			Timestamp: s.StartedAt,
			Type:      "session",
			Summary:   fmt.Sprintf("%s (focus: %.0f%%, app: %s)", s.SessionType, s.FocusScore*100, s.DominantApp),
		})
	}

	// Screenshots.
	screenshots, err := store.GetScreenshotsInRange(db, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("timeline screenshots: %w", err)
	}
	for _, s := range screenshots {
		entries = append(entries, TimelineEntry{
			Timestamp: s.CapturedAt,
			Type:      "screenshot",
			Summary:   fmt.Sprintf("[%s] %s — %s", s.SourceApp, s.WindowTitle, s.FilePath),
		})
	}

	// Intent traces.
	traces, err := store.GetTracesInRange(db, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("timeline traces: %w", err)
	}
	for _, t := range traces {
		entries = append(entries, TimelineEntry{
			Timestamp: t.CompletedAt,
			Type:      "trace",
			Summary:   fmt.Sprintf("%s: %s", t.OutcomeType, t.OutcomeDetail),
		})
	}

	// Live Vercel runtime logs (live:vercel source).
	vlogs, err := store.GetVercelLogsInRange(db, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("timeline vercel: %w", err)
	}
	for _, v := range vlogs {
		meta := v.Level
		if v.Path != "" || v.StatusCode != 0 {
			meta = strings.TrimSpace(fmt.Sprintf("%s %s %s %d", v.Level, v.Method, v.Path, v.StatusCode))
		}
		entries = append(entries, TimelineEntry{
			Timestamp: v.CapturedAt,
			Type:      "live:vercel",
			Summary:   fmt.Sprintf("[%s] %s", strings.TrimSpace(meta), v.Message),
		})
	}

	// Sort by timestamp ascending.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})

	return entries, nil
}

// FormatTimeline formats timeline entries.
func FormatTimeline(entries []TimelineEntry, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(entries, "", "  ")
		return string(b)
	case "md":
		return formatTimelineMD(entries)
	default:
		return formatTimelineHuman(entries)
	}
}

func formatTimelineHuman(entries []TimelineEntry) string {
	if len(entries) == 0 {
		return "no events in this time range"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d events\n\n", len(entries)))

	lastDate := ""
	for _, e := range entries {
		ts := time.Unix(e.Timestamp, 0)
		date := ts.Format("2006-01-02")
		if date != lastDate {
			sb.WriteString(fmt.Sprintf("--- %s ---\n", date))
			lastDate = date
		}

		icon := typeIcon(e.Type)
		sb.WriteString(fmt.Sprintf("  %s %s %s\n", ts.Format("15:04:05"), icon, e.Summary))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatTimelineMD(entries []TimelineEntry) string {
	if len(entries) == 0 {
		return "## Timeline\n\n_No events._\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Timeline (%d events)\n\n", len(entries)))

	lastDate := ""
	for _, e := range entries {
		ts := time.Unix(e.Timestamp, 0)
		date := ts.Format("2006-01-02")
		if date != lastDate {
			sb.WriteString(fmt.Sprintf("### %s\n\n", date))
			lastDate = date
		}

		sb.WriteString(fmt.Sprintf("- **%s** `%s` %s\n", ts.Format("15:04:05"), e.Type, e.Summary))
	}
	return sb.String()
}

func typeIcon(t string) string {
	switch t {
	case "activity":
		return "[app]"
	case "clipboard":
		return "[clip]"
	case "session":
		return "[sess]"
	case "screenshot":
		return "[snap]"
	case "trace":
		return "[TRACE]"
	case "live:vercel":
		return "[vercel]"
	default:
		return "[?]"
	}
}
