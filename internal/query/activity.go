package query

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sksareen/sauron/internal/store"
)

// ActivitySummary is an aggregated view of activity over a time window.
type ActivitySummary struct {
	Hours        float64            `json:"hours"`
	FocusScore   float64            `json:"focus_score"`
	AppBreakdown map[string]float64 `json:"app_breakdown"` // app -> hours
	TotalApps    int                `json:"total_apps"`
	Switches     int                `json:"switches"`
}

// GetActivity returns an activity summary for the last h hours.
func GetActivity(db *store.DB, hours float64) (*ActivitySummary, error) {
	entries, err := store.GetActivity(db, hours)
	if err != nil {
		return nil, err
	}

	summary := &ActivitySummary{
		Hours:        hours,
		AppBreakdown: make(map[string]float64),
	}

	apps := make(map[string]struct{})
	for _, e := range entries {
		ms := e.DurationMs
		if ms == 0 {
			ms = 5000 // estimate 5s for still-running entries
		}
		hours := float64(ms) / 3600000.0
		summary.AppBreakdown[e.AppName] += hours
		apps[e.AppName] = struct{}{}
	}

	summary.TotalApps = len(apps)
	summary.Switches = len(entries) - 1
	if summary.Switches < 0 {
		summary.Switches = 0
	}
	summary.FocusScore = focusScoreFromSwitches(summary.Switches)

	return summary, nil
}

// FormatActivity formats an activity summary.
func FormatActivity(s *ActivitySummary, format string) string {
	switch format {
	case "json":
		b, _ := json.MarshalIndent(s, "", "  ")
		return string(b)
	case "md":
		return formatActivityMD(s)
	default:
		return formatActivityHuman(s)
	}
}

func formatActivityHuman(s *ActivitySummary) string {
	if len(s.AppBreakdown) == 0 {
		return fmt.Sprintf("no activity in the last %.0f hours", s.Hours)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("last %.0fh  focus: %.0f%%  apps: %d  switches: %d\n\n",
		s.Hours, s.FocusScore*100, s.TotalApps, s.Switches))

	total := totalHours(s.AppBreakdown)
	for _, kv := range sortedByValue(s.AppBreakdown) {
		bar := progressBar(kv.Value, total, 20)
		h := int(kv.Value)
		m := int((kv.Value - float64(h)) * 60)
		dur := fmt.Sprintf("%dh %dm", h, m)
		if h == 0 {
			dur = fmt.Sprintf("%dm", m)
		}
		sb.WriteString(fmt.Sprintf("  %-30s %s  %s\n", kv.Key, bar, dur))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatActivityMD(s *ActivitySummary) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Activity (last %.0fh)\n\n", s.Hours))
	sb.WriteString(fmt.Sprintf("- **Focus score**: %.0f%%\n", s.FocusScore*100))
	sb.WriteString(fmt.Sprintf("- **App switches**: %d\n", s.Switches))
	sb.WriteString(fmt.Sprintf("- **Unique apps**: %d\n\n", s.TotalApps))
	sb.WriteString("| App | Minutes |\n|---|---|\n")
	for _, kv := range sortedByValue(s.AppBreakdown) {
		sb.WriteString(fmt.Sprintf("| %s | %.0f |\n", kv.Key, kv.Value))
	}
	return sb.String()
}

func focusScoreFromSwitches(switches int) float64 {
	score := 1.0 - float64(switches)/10.0
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func totalHours(breakdown map[string]float64) float64 {
	var total float64
	for _, v := range breakdown {
		total += v
	}
	return total
}

func progressBar(value, total float64, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	filled := int(value / total * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// sortedByValue returns app/minutes pairs sorted descending by value.
func sortedByValue(m map[string]float64) []struct {
	Key   string
	Value float64
} {
	type kv struct {
		Key   string
		Value float64
	}
	var sorted []kv
	for k, v := range m {
		sorted = append(sorted, kv{k, v})
	}
	// Insertion sort (small N).
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Value > sorted[j-1].Value; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	result := make([]struct {
		Key   string
		Value float64
	}, len(sorted))
	for i, kv := range sorted {
		result[i].Key = kv.Key
		result[i].Value = kv.Value
	}
	return result
}
