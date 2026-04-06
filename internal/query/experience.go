package query

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sksareen/sauron/internal/embed"
	"github.com/sksareen/sauron/internal/store"
)

// ExperienceResult pairs a record with its similarity score.
type ExperienceResult struct {
	Record store.ExperienceRecord `json:"record"`
	Score  float64                `json:"score"`
}

// CheckExperience performs semantic search over experiences, falling back to text search.
func CheckExperience(db *store.DB, taskDescription, context string, limit int) ([]ExperienceResult, int, error) {
	count, err := store.GetExperienceCount(db)
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, 0, nil
	}

	query := taskDescription
	if context != "" {
		query = taskDescription + "\n\nContext: " + context
	}

	// Try semantic search first.
	queryEmb, embErr := embed.GetEmbedding(query)
	if embErr == nil && queryEmb != nil {
		allWithEmb, err := store.GetExperiencesWithEmbeddings(db)
		if err != nil {
			return nil, count, err
		}

		var results []ExperienceResult
		for _, rec := range allWithEmb {
			recVec := embed.BytesToVector(rec.Embedding)
			if recVec == nil {
				continue
			}
			score := embed.CosineSimilarity(queryEmb, recVec)
			results = append(results, ExperienceResult{Record: rec, Score: score})
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

		if len(results) > limit {
			results = results[:limit]
		}
		return results, count, nil
	}

	// Fallback: text search.
	textResults, err := store.SearchExperiencesByText(db, taskDescription, limit)
	if err != nil {
		return nil, count, err
	}
	var results []ExperienceResult
	for _, r := range textResults {
		results = append(results, ExperienceResult{Record: r, Score: 0})
	}
	return results, count, nil
}

// FormatCheckExperience formats search results for display.
func FormatCheckExperience(results []ExperienceResult, total int, format string) string {
	if len(results) == 0 {
		if total == 0 {
			return "No experiences in the graph yet. Use sauron_log_experience to record your first."
		}
		return fmt.Sprintf("No relevant experiences found among %d records.", total)
	}

	if format == "json" {
		b, _ := json.MarshalIndent(results, "", "  ")
		return string(b)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant experiences (%d total in graph):\n\n", len(results), total))

	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		rec := r.Record
		if r.Score > 0 {
			sb.WriteString(fmt.Sprintf("## Experience %d (relevance: %.1f%%)\n", i+1, r.Score*100))
		} else {
			sb.WriteString(fmt.Sprintf("## Experience %d\n", i+1))
		}
		sb.WriteString(fmt.Sprintf("**Task:** %s\n", rec.TaskIntent))
		sb.WriteString(fmt.Sprintf("**Approach:** %s\n", rec.Approach))
		sb.WriteString(fmt.Sprintf("**Outcome:** %s\n", rec.Outcome))
		if len(rec.ToolsUsed) > 0 {
			sb.WriteString(fmt.Sprintf("**Tools:** %s\n", strings.Join(rec.ToolsUsed, ", ")))
		}
		if len(rec.FailurePoints) > 0 {
			sb.WriteString(fmt.Sprintf("**Failure points:** %s\n", strings.Join(rec.FailurePoints, "; ")))
		}
		if rec.Resolution != "" {
			sb.WriteString(fmt.Sprintf("**Resolution:** %s\n", rec.Resolution))
		}
		if len(rec.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(rec.Tags, ", ")))
		}
	}
	return sb.String()
}

// FormatExperienceStats formats experience graph statistics.
func FormatExperienceStats(total, success, failure, partial int, format string) string {
	if format == "json" {
		b, _ := json.Marshal(map[string]int{
			"total":   total,
			"success": success,
			"failure": failure,
			"partial": partial,
		})
		return string(b)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Agent experience graph: %d total records\n", total))
	if total > 0 {
		sb.WriteString(fmt.Sprintf("  success: %d (%.0f%%)\n", success, pct(success, total)))
		sb.WriteString(fmt.Sprintf("  failure: %d (%.0f%%)\n", failure, pct(failure, total)))
		sb.WriteString(fmt.Sprintf("  partial: %d (%.0f%%)\n", partial, pct(partial, total)))
	}
	return sb.String()
}

// FormatRecentExperiences formats a list of recent experiences.
func FormatRecentExperiences(records []store.ExperienceRecord, format string) string {
	if len(records) == 0 {
		return "No experiences recorded yet."
	}

	if format == "json" {
		b, _ := json.MarshalIndent(records, "", "  ")
		return string(b)
	}

	var sb strings.Builder
	for i, rec := range records {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(fmt.Sprintf("## #%d [%s] %s\n", rec.ID, rec.Outcome, rec.TaskIntent))
		sb.WriteString(fmt.Sprintf("**Approach:** %s\n", rec.Approach))
		if len(rec.ToolsUsed) > 0 {
			sb.WriteString(fmt.Sprintf("**Tools:** %s\n", strings.Join(rec.ToolsUsed, ", ")))
		}
		if len(rec.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(rec.Tags, ", ")))
		}
		if rec.CreatedAt != "" {
			sb.WriteString(fmt.Sprintf("**When:** %s\n", rec.CreatedAt))
		}
	}
	return sb.String()
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
