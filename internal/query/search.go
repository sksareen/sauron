package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// Search runs a full-text search across clipboard content.
func Search(db *store.DB, query string) ([]store.SearchResult, error) {
	return store.SearchAll(db, query, 20)
}

// FormatSearch formats search results.
func FormatSearch(results []store.SearchResult, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(results, "", "  ")
		return string(b)
	case "md":
		return formatSearchMD(results)
	default:
		return formatSearchHuman(results)
	}
}

func formatSearchHuman(results []store.SearchResult) string {
	if len(results) == 0 {
		return "no results"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d result(s)\n\n", len(results)))
	for i, r := range results {
		ts := time.Unix(r.CapturedAt, 0).Format("2006-01-02 15:04")
		app := r.SourceApp
		if app == "" {
			app = "unknown"
		}
		content := r.Content
		if len(content) > 200 {
			content = content[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] %s (%s)\n", i+1, ts, app))
		sb.WriteString(fmt.Sprintf("    %s\n\n", content))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatSearchMD(results []store.SearchResult) string {
	if len(results) == 0 {
		return "## Search Results\n\n_No results._\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Search Results (%d)\n\n", len(results)))
	for _, r := range results {
		ts := time.Unix(r.CapturedAt, 0).Format("2006-01-02 15:04")
		app := r.SourceApp
		if app == "" {
			app = "unknown"
		}
		content := strings.ReplaceAll(r.Content, "`", "'")
		if len(content) > 300 {
			content = content[:297] + "..."
		}
		sb.WriteString(fmt.Sprintf("### %s — %s\n\n```\n%s\n```\n\n", ts, app, content))
	}
	return sb.String()
}
