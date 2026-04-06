package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// GetTraces returns the most recent n intent traces.
func GetTraces(db *store.DB, n int) ([]store.IntentTrace, error) {
	return store.GetRecentTraces(db, n)
}

// FormatTraces formats intent traces.
func FormatTraces(traces []store.IntentTrace, format string) string {
	switch format {
	case "json":
		// Strip raw_events and embedding for readability.
		type traceView struct {
			ID            int64  `json:"id"`
			OutcomeType   string `json:"outcome_type"`
			OutcomeDetail string `json:"outcome_detail"`
			TraceSummary  string `json:"trace_summary"`
			WindowMin     int    `json:"activity_window_minutes"`
			StartedAt     int64  `json:"started_at"`
			CompletedAt   int64  `json:"completed_at"`
		}
		views := make([]traceView, len(traces))
		for i, t := range traces {
			views[i] = traceView{
				ID:            t.ID,
				OutcomeType:   t.OutcomeType,
				OutcomeDetail: t.OutcomeDetail,
				TraceSummary:  t.TraceSummary,
				WindowMin:     t.ActivityWindowMinutes,
				StartedAt:     t.StartedAt,
				CompletedAt:   t.CompletedAt,
			}
		}
		b, _ := json.MarshalIndent(views, "", "  ")
		return string(b)
	case "md":
		return formatTracesMD(traces)
	default:
		return formatTracesHuman(traces)
	}
}

func formatTracesHuman(traces []store.IntentTrace) string {
	if len(traces) == 0 {
		return "no intent traces recorded yet"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d trace(s)\n\n", len(traces)))
	for i, t := range traces {
		ts := time.Unix(t.CompletedAt, 0).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("[%d] %s  %s\n", i+1, ts, t.OutcomeType))
		detail := t.OutcomeDetail
		if len(detail) > 120 {
			detail = detail[:117] + "..."
		}
		sb.WriteString(fmt.Sprintf("    %s\n", detail))
		summary := t.TraceSummary
		if len(summary) > 200 {
			summary = summary[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("    %s\n\n", summary))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatTracesMD(traces []store.IntentTrace) string {
	if len(traces) == 0 {
		return "## Intent Traces\n\n_No traces recorded._\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Intent Traces (%d)\n\n", len(traces)))
	for _, t := range traces {
		ts := time.Unix(t.CompletedAt, 0).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("### %s — %s\n\n", ts, t.OutcomeType))
		sb.WriteString(fmt.Sprintf("**%s**\n\n", t.OutcomeDetail))
		summary := t.TraceSummary
		if len(summary) > 500 {
			summary = summary[:497] + "..."
		}
		sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", summary))
	}
	return sb.String()
}
