package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// GetClipboard returns the last n clipboard items.
func GetClipboard(db *store.DB, n int) ([]store.ClipboardItem, error) {
	return store.GetRecentClipboard(db, n)
}

// FormatClipboard formats clipboard items.
// format: "human" | "json" | "md"
func FormatClipboard(items []store.ClipboardItem, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(items, "", "  ")
		return string(b)
	case "md":
		return formatClipboardMD(items)
	default:
		return formatClipboardHuman(items)
	}
}

func formatClipboardHuman(items []store.ClipboardItem) string {
	if len(items) == 0 {
		return "no clipboard items captured yet"
	}
	var sb strings.Builder
	for i, item := range items {
		ts := time.Unix(item.CapturedAt, 0).Format("15:04:05")
		content := item.Content
		if len(content) > 120 {
			content = content[:117] + "..."
		}
		app := item.SourceApp
		if app == "" {
			app = "unknown"
		}
		sb.WriteString(fmt.Sprintf("[%d] %s (%s)\n    %s\n", i+1, ts, app, content))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatClipboardMD(items []store.ClipboardItem) string {
	if len(items) == 0 {
		return "## Clipboard\n\n_No items._\n"
	}
	var sb strings.Builder
	sb.WriteString("## Clipboard\n\n")
	for _, item := range items {
		ts := time.Unix(item.CapturedAt, 0).Format("15:04:05")
		app := item.SourceApp
		if app == "" {
			app = "unknown"
		}
		content := strings.ReplaceAll(item.Content, "`", "'")
		if len(content) > 200 {
			content = content[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s): `%s`\n", ts, app, content))
	}
	return sb.String()
}
