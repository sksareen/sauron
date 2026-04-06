package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// GetScreenshots returns the last n screenshots.
func GetScreenshots(db *store.DB, n int) ([]store.Screenshot, error) {
	return store.GetRecentScreenshots(db, n)
}

// FormatScreenshots formats screenshot entries.
func FormatScreenshots(items []store.Screenshot, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(items, "", "  ")
		return string(b)
	default:
		return formatScreenshotsHuman(items)
	}
}

func formatScreenshotsHuman(items []store.Screenshot) string {
	if len(items) == 0 {
		return "no screenshots captured yet"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d screenshots\n\n", len(items)))
	for i, s := range items {
		ts := time.Unix(s.CapturedAt, 0).Format("15:04:05")
		app := s.SourceApp
		if app == "" {
			app = "unknown"
		}
		title := s.WindowTitle
		if title == "" {
			title = app
		}
		sb.WriteString(fmt.Sprintf("[%d] %s  %s — %s\n    %s\n", i+1, ts, app, title, s.FilePath))
	}
	return strings.TrimRight(sb.String(), "\n")
}
