package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// HintSummary is the UI-facing view of a HINT with its top evidence.
type HintSummary struct {
	store.HintRecord
	Evidence []store.HintEvidence `json:"evidence"`
}

// GetRecentHints returns all hints (any status) with evidence, for the history view.
func GetRecentHints(db *store.DB, limit int) ([]HintSummary, error) {
	hints, err := store.GetRecentHints(db, limit)
	if err != nil {
		return nil, err
	}
	out := make([]HintSummary, 0, len(hints))
	for _, h := range hints {
		ev, _ := store.GetHintEvidence(db, h.ID, 20)
		out = append(out, HintSummary{HintRecord: h, Evidence: ev})
	}
	return out, nil
}

// GetHints returns the top N active hints with their recent evidence.
func GetHints(db *store.DB, limit int) ([]HintSummary, error) {
	hints, err := store.GetActiveHints(db, limit)
	if err != nil {
		return nil, err
	}

	out := make([]HintSummary, 0, len(hints))
	for _, h := range hints {
		ev, _ := store.GetHintEvidence(db, h.ID, 6)
		out = append(out, HintSummary{HintRecord: h, Evidence: ev})
	}
	return out, nil
}

// FormatHints formats hints for human or JSON output.
func FormatHints(hints []HintSummary, format string) string {
	if format == "json" {
		b, _ := json.MarshalIndent(hints, "", "  ")
		return string(b)
	}

	if len(hints) == 0 {
		return "no active hints. keep working — hints appear after ~2 minutes of activity."
	}

	var sb strings.Builder
	for i, h := range hints {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		label := h.Label
		if label == "" {
			label = fmt.Sprintf("unlabelled (%s)", h.DominantApp)
		}
		age := fmtHintAge(h.StartedAt)
		sb.WriteString(fmt.Sprintf("hint[%d]  weight=%.2f  %s\n", i+1, h.Weight, statusBadge(h.Status)))
		sb.WriteString(fmt.Sprintf("label:   %s (confidence: %.0f%%)\n", label, h.Confidence*100))
		sb.WriteString(fmt.Sprintf("app:     %s\n", h.DominantApp))
		sb.WriteString(fmt.Sprintf("age:     %s  evidence: %d rows\n", age, h.EvidenceCount))
		if len(h.Evidence) > 0 {
			sb.WriteString("recent evidence:\n")
			for _, e := range h.Evidence {
				ts := time.Unix(e.Ts, 0).Format("15:04:05")
				sb.WriteString(fmt.Sprintf("  %s  %s\n", ts, e.Summary))
			}
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func fmtHintAge(startedAt int64) string {
	sec := time.Now().Unix() - startedAt
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	if min < 60 {
		return fmt.Sprintf("%dm", min)
	}
	return fmt.Sprintf("%dh %dm", min/60, min%60)
}

func statusBadge(status string) string {
	switch status {
	case "active":
		return "[active]"
	case "paused":
		return "[paused]"
	default:
		return fmt.Sprintf("[%s]", status)
	}
}
