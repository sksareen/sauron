package query

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// Reentry is the open-loop payload Valinor's activity view reads.
type Reentry struct {
	Task    *ReentryTask    `json:"task"`
	Project *ReentryProject `json:"project"`
	Trace   *ReentryTrace   `json:"trace"`
}

// ReentryTask is one inferred unfinished thread.
type ReentryTask struct {
	TaskID          string `json:"task_id"`
	Status          string `json:"status"`
	Goal            string `json:"goal"`
	NextAction      string `json:"next_action,omitempty"`
	StartedAt       int64  `json:"started_at,omitempty"`
	UpdatedAt       int64  `json:"updated_at,omitempty"`
	LastUsefulState string `json:"last_useful_state,omitempty"`
}

// ReentryProject names the app or repo the thread belongs to.
type ReentryProject struct {
	Name string `json:"name"`
}

// ReentryTrace is the latest outcome attached to the open thread.
type ReentryTrace struct {
	Summary    string `json:"summary,omitempty"`
	NextAction string `json:"next_action,omitempty"`
}

// ValinorTaskID reports whether id matches the token Valinor will post back.
func ValinorTaskID(id string) bool {
	if len(id) < 4 || len(id) > 96 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.' || r == ':' || r == '-':
		default:
			return false
		}
	}
	return true
}

// BuildReentry infers the unfinished thread Valinor should offer to mark done.
// A task the user already completed stays hidden.
func BuildReentry(db *store.DB, now time.Time) (*Reentry, error) {
	task, err := inferTask(db, now)
	if err != nil {
		return nil, err
	}
	empty := &Reentry{}
	if task == nil {
		return empty, nil
	}
	done, err := store.TaskCompleted(db, task.Task.TaskID)
	if err != nil {
		return nil, err
	}
	if done {
		return empty, nil
	}
	return task, nil
}

// OpenThreadFields returns the goal and next step for context JSON.
// It does not consult completed_tasks, so a read-only database without that
// table still answers.
func OpenThreadFields(db *store.DB, now time.Time) (thread, next string) {
	task, err := inferTask(db, now)
	if err != nil || task == nil || task.Task == nil {
		return "", ""
	}
	return task.Task.Goal, task.Task.NextAction
}

func inferTask(db *store.DB, now time.Time) (*Reentry, error) {
	session, err := store.GetCurrentSession(db)
	if err != nil {
		return nil, err
	}
	traces, err := store.GetRecentTraces(db, 1)
	if err != nil {
		return nil, err
	}

	var trace *store.IntentTrace
	if len(traces) > 0 {
		trace = &traces[0]
	}

	if trace != nil && now.Sub(time.Unix(trace.CompletedAt, 0)) <= 6*time.Hour {
		goal := firstLine(trace.OutcomeDetail)
		if goal == "" {
			goal = firstLine(trace.TraceSummary)
		}
		if goal != "" {
			project := trace.OutcomeType
			if project == "" {
				project = "trace"
			}
			next := ""
			if session != nil && session.SessionType != "" {
				next = strings.ReplaceAll(session.SessionType, "_", " ")
			}
			return &Reentry{
				Task: &ReentryTask{
					TaskID:          fmt.Sprintf("trace-%d", trace.ID),
					Status:          "active",
					Goal:            goal,
					NextAction:      next,
					StartedAt:       trace.StartedAt,
					UpdatedAt:       trace.CompletedAt,
					LastUsefulState: clip(trace.TraceSummary, 220),
				},
				Project: &ReentryProject{Name: project},
				Trace: &ReentryTrace{
					Summary:    clip(trace.TraceSummary, 220),
					NextAction: next,
				},
			}, nil
		}
	}

	if session != nil && session.DominantApp != "" {
		next := strings.ReplaceAll(session.SessionType, "_", " ")
		return &Reentry{
			Task: &ReentryTask{
				TaskID:          fmt.Sprintf("session-%d", session.ID),
				Status:          "active",
				Goal:            "Continue in " + session.DominantApp,
				NextAction:      next,
				StartedAt:       session.StartedAt,
				UpdatedAt:       session.StartedAt,
				LastUsefulState: strings.TrimSpace(next + " in " + session.DominantApp),
			},
			Project: &ReentryProject{Name: session.DominantApp},
			Trace:   &ReentryTrace{NextAction: next},
		}, nil
	}

	return nil, nil
}

// FormatReentry renders a reentry payload.
func FormatReentry(r *Reentry, format string) string {
	if r == nil {
		r = &Reentry{}
	}
	if format == "json" {
		b, _ := json.MarshalIndent(r, "", "  ")
		return string(b)
	}
	if r.Task == nil || r.Task.Status != "active" {
		return "no open thread"
	}
	return fmt.Sprintf("%s\n%s", r.Task.Goal, r.Task.NextAction)
}

// FormatHints renders manual hints. JSON is always an array.
func FormatHints(hints []store.ManualHint, format string) string {
	if hints == nil {
		hints = []store.ManualHint{}
	}
	if format == "json" {
		b, _ := json.MarshalIndent(hints, "", "  ")
		return string(b)
	}
	if len(hints) == 0 {
		return "no hints"
	}
	var sb strings.Builder
	for _, h := range hints {
		if h.Status != "active" || h.Label == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s  %s\n", h.ID, h.Label))
	}
	out := strings.TrimRight(sb.String(), "\n")
	if out == "" {
		return "no hints"
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(s, 140)
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
