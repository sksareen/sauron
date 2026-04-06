package query

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/embed"
	"github.com/sksareen/sauron/internal/store"
)

// RecallResult is a single semantic search hit from intent traces.
type RecallResult struct {
	OutcomeType   string  `json:"outcome_type"`
	OutcomeDetail string  `json:"outcome_detail"`
	TraceSummary  string  `json:"trace_summary"`
	CompletedAt   int64   `json:"completed_at"`
	Similarity    float64 `json:"similarity"`
}

// Recall performs semantic search over intent traces.
// hours defaults to 168 (1 week) if <= 0.
func Recall(db *store.DB, query string, hours float64) ([]RecallResult, error) {
	if hours <= 0 {
		hours = 168
	}

	sinceUnix := time.Now().Unix() - int64(hours*3600)

	// Embed the query.
	queryVec, err := embed.GetEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// If no embedding available (no API key), fall back to text matching.
	if queryVec == nil {
		return recallFallback(db, query, sinceUnix)
	}

	// Get all traces with embeddings in the time range.
	traces, err := store.GetTracesWithEmbedding(db, sinceUnix)
	if err != nil {
		return nil, fmt.Errorf("get traces: %w", err)
	}

	if len(traces) == 0 {
		return nil, nil
	}

	// Compute similarity scores.
	type scored struct {
		trace store.IntentTrace
		sim   float64
	}
	var results []scored
	for _, t := range traces {
		traceVec := embed.BytesToVector(t.Embedding)
		if traceVec == nil {
			continue
		}
		sim := embed.CosineSimilarity(queryVec, traceVec)
		results = append(results, scored{trace: t, sim: sim})
	}

	// Sort by similarity descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].sim > results[j].sim
	})

	// Take top 5.
	limit := 5
	if len(results) < limit {
		limit = len(results)
	}

	var out []RecallResult
	for _, r := range results[:limit] {
		out = append(out, RecallResult{
			OutcomeType:   r.trace.OutcomeType,
			OutcomeDetail: r.trace.OutcomeDetail,
			TraceSummary:  r.trace.TraceSummary,
			CompletedAt:   r.trace.CompletedAt,
			Similarity:    r.sim,
		})
	}
	return out, nil
}

// recallFallback does simple text matching when embeddings are unavailable.
func recallFallback(db *store.DB, query string, sinceUnix int64) ([]RecallResult, error) {
	traces, err := store.GetTracesInRange(db, sinceUnix, time.Now().Unix())
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var out []RecallResult
	for _, t := range traces {
		text := strings.ToLower(t.TraceSummary + " " + t.OutcomeDetail)
		if strings.Contains(text, queryLower) {
			out = append(out, RecallResult{
				OutcomeType:   t.OutcomeType,
				OutcomeDetail: t.OutcomeDetail,
				TraceSummary:  t.TraceSummary,
				CompletedAt:   t.CompletedAt,
				Similarity:    0.5, // arbitrary for text match
			})
		}
		if len(out) >= 5 {
			break
		}
	}
	return out, nil
}

// FormatRecall formats recall results.
func FormatRecall(results []RecallResult, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(results, "", "  ")
		return string(b)
	case "md":
		return formatRecallMD(results)
	default:
		return formatRecallHuman(results)
	}
}

func formatRecallHuman(results []RecallResult) string {
	if len(results) == 0 {
		return "no matching traces found"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d trace(s) found\n\n", len(results)))
	for i, r := range results {
		ts := time.Unix(r.CompletedAt, 0).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("[%d] %s  (%.0f%% match)\n", i+1, ts, r.Similarity*100))
		sb.WriteString(fmt.Sprintf("    %s: %s\n", r.OutcomeType, r.OutcomeDetail))
		summary := r.TraceSummary
		if len(summary) > 200 {
			summary = summary[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("    %s\n\n", summary))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatRecallMD(results []RecallResult) string {
	if len(results) == 0 {
		return "## Recall\n\n_No matching traces._\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Recall (%d traces)\n\n", len(results)))
	for _, r := range results {
		ts := time.Unix(r.CompletedAt, 0).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("### %s — %s (%.0f%% match)\n\n", ts, r.OutcomeType, r.Similarity*100))
		sb.WriteString(fmt.Sprintf("**%s**\n\n", r.OutcomeDetail))
		summary := r.TraceSummary
		if len(summary) > 500 {
			summary = summary[:497] + "..."
		}
		sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", summary))
	}
	return sb.String()
}
