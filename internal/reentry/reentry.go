package reentry

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/scrub"
	"github.com/sksareen/sauron/internal/store"
)

const (
	DefaultLookbackMinutes = 30
	DriftThresholdSeconds  = 180
)

// LoopContext is the current inferred open loop.
type LoopContext struct {
	Project       *store.Project  `json:"project,omitempty"`
	Task          *store.OpenTask `json:"task,omitempty"`
	Confidence    float64         `json:"confidence"`
	DriftDetected bool            `json:"drift_detected"`
	Reason        string          `json:"reason,omitempty"`
}

// ReentryCard is the soft nudge payload shown to humans and agents.
type ReentryCard struct {
	Project     *store.Project          `json:"project,omitempty"`
	Task        *store.OpenTask         `json:"task,omitempty"`
	Trace       *store.HumanTrace       `json:"trace,omitempty"`
	Events      []store.HumanTraceEvent `json:"events,omitempty"`
	Reason      string                  `json:"reason"`
	NextAction  string                  `json:"next_action"`
	Confidence  float64                 `json:"confidence"`
	GeneratedAt int64                   `json:"generated_at"`
}

// TraceExplanation is a readable v2 trace with linked evidence.
type TraceExplanation struct {
	Trace   *store.HumanTrace       `json:"trace,omitempty"`
	Project *store.Project          `json:"project,omitempty"`
	Task    *store.OpenTask         `json:"task,omitempty"`
	Events  []store.HumanTraceEvent `json:"events,omitempty"`
}

type inference struct {
	projectKey      string
	projectName     string
	projectKind     string
	rootHint        string
	goal            string
	lastUsefulState string
	nextAction      string
	confidence      float64
	activity        *store.ActivityEntry
}

// Evaluate updates the parallel v2 task/trace model from existing evidence.
func Evaluate(db *store.DB, now int64) (*LoopContext, error) {
	if now == 0 {
		now = time.Now().Unix()
	}

	active, err := store.GetActiveOpenTask(db)
	if err != nil {
		return nil, err
	}

	activities, sessions, clipboards, screenshots := evidenceWindow(db, now)
	inf := inferLoop(activities, sessions, clipboards, now)

	var project *store.Project
	if inf != nil && inf.confidence >= 0.45 {
		project = &store.Project{
			ProjectKey:   inf.projectKey,
			Name:         inf.projectName,
			Kind:         inf.projectKind,
			RootHint:     inf.rootHint,
			CreatedAt:    now,
			UpdatedAt:    now,
			MetadataJSON: metadata(map[string]string{"source": "inferred"}),
		}
		projectID, err := store.UpsertProject(db, project)
		if err != nil {
			return nil, err
		}
		project.ID = projectID

		if active == nil || active.Status == "completed" || active.Status == "abandoned" || active.ProjectID != projectID {
			active = &store.OpenTask{
				TaskID:          newID("task", now),
				ProjectID:       projectID,
				Status:          "active",
				Goal:            inf.goal,
				LastUsefulState: inf.lastUsefulState,
				NextAction:      inf.nextAction,
				Confidence:      inf.confidence,
				StartedAt:       now,
				UpdatedAt:       now,
				MetadataJSON:    metadata(map[string]string{"source": "inferred"}),
			}
			if _, err := store.UpsertOpenTask(db, active); err != nil {
				return nil, err
			}
		} else if !currentActivityIsDistraction(activities, now) {
			active.Status = "active"
			active.Goal = inf.goal
			active.LastUsefulState = inf.lastUsefulState
			active.NextAction = inf.nextAction
			active.Confidence = maxFloat(active.Confidence, inf.confidence)
			active.UpdatedAt = now
			if _, err := store.UpsertOpenTask(db, active); err != nil {
				return nil, err
			}
		}
	} else if active != nil && active.ProjectID != 0 {
		project, _ = store.GetProjectByID(db, active.ProjectID)
	}

	drift, reason := detectDrift(activities, now)
	if active != nil && drift {
		if err := createInterruptionTrace(db, active, project, activities, sessions, clipboards, screenshots, now, reason); err != nil {
			return nil, err
		}
		_ = store.UpdateOpenTaskStatus(db, active.TaskID, "paused", "drift", now, 0)
		active.Status = "paused"
		active.UpdatedAt = now
	}

	conf := 0.0
	if active != nil {
		conf = active.Confidence
	}
	return &LoopContext{
		Project:       project,
		Task:          active,
		Confidence:    conf,
		DriftDetected: drift,
		Reason:        reason,
	}, nil
}

// CurrentLoop returns the best known v2 loop without mutating old data.
func CurrentLoop(db *store.DB) (*LoopContext, error) {
	task, err := store.GetActiveOpenTask(db)
	if err != nil {
		return nil, err
	}
	var project *store.Project
	if task != nil && task.ProjectID != 0 {
		project, _ = store.GetProjectByID(db, task.ProjectID)
	}
	conf := 0.0
	if task != nil {
		conf = task.Confidence
	}
	return &LoopContext{Project: project, Task: task, Confidence: conf}, nil
}

// MarkTask creates or updates the current open loop from explicit user/agent intent.
func MarkTask(db *store.DB, goal, status, projectHint string) (*store.OpenTask, error) {
	now := time.Now().Unix()
	status = normalizeTaskStatus(status)
	if status == "" {
		status = "active"
	}

	active, err := store.GetActiveOpenTask(db)
	if err != nil {
		return nil, err
	}

	projectName := strings.TrimSpace(projectHint)
	if projectName == "" && active != nil && active.ProjectID != 0 {
		project, _ := store.GetProjectByID(db, active.ProjectID)
		if project != nil {
			projectName = project.Name
		}
	}
	if projectName == "" {
		projectName = "manual"
	}

	project := &store.Project{
		ProjectKey:   "manual:" + slug(projectName),
		Name:         projectName,
		Kind:         "manual",
		CreatedAt:    now,
		UpdatedAt:    now,
		MetadataJSON: metadata(map[string]string{"source": "manual"}),
	}
	projectID, err := store.UpsertProject(db, project)
	if err != nil {
		return nil, err
	}

	taskID := newID("task", now)
	startedAt := now
	if active != nil && active.Status != "completed" && active.Status != "abandoned" {
		taskID = active.TaskID
		startedAt = active.StartedAt
	}

	task := &store.OpenTask{
		TaskID:          taskID,
		ProjectID:       projectID,
		Status:          status,
		Goal:            scrubShort(goal, 240),
		LastUsefulState: "Marked explicitly by user or agent.",
		NextAction:      nextActionFromGoal(goal),
		Confidence:      1,
		StartedAt:       startedAt,
		UpdatedAt:       now,
		MetadataJSON:    metadata(map[string]string{"source": "manual"}),
	}
	if status == "completed" || status == "abandoned" {
		task.CompletedAt = now
		task.CompletionSource = "manual"
	}
	if _, err := store.UpsertOpenTask(db, task); err != nil {
		return nil, err
	}

	traceType := "reentry"
	if status == "completed" {
		traceType = "completion"
	}
	trace := &store.HumanTrace{
		TraceID:     newID("trace", now),
		TraceType:   traceType,
		ProjectID:   projectID,
		TaskID:      task.TaskID,
		TriggerType: "manual_marker",
		Status:      status,
		StartedAt:   task.StartedAt,
		CompletedAt: now,
		Summary:     scrubShort(fmt.Sprintf("Manual task marker: %s", goal), 360),
		NextAction:  task.NextAction,
		Confidence:  1,
		MetadataJSON: metadata(map[string]string{
			"privacy": "summaries_only",
		}),
	}
	if _, err := store.InsertHumanTrace(db, trace); err != nil {
		return nil, err
	}
	return task, nil
}

// CompleteTask marks a v2 task complete and writes a v2 completion trace.
func CompleteTask(db *store.DB, taskID string) (*store.OpenTask, error) {
	now := time.Now().Unix()
	var task *store.OpenTask
	var err error
	if taskID != "" {
		task, err = store.GetOpenTaskByTaskID(db, taskID)
	} else {
		task, err = store.GetActiveOpenTask(db)
	}
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("no active v2 task found")
	}
	if err := store.UpdateOpenTaskStatus(db, task.TaskID, "completed", "manual", now, now); err != nil {
		return nil, err
	}
	task.Status = "completed"
	task.UpdatedAt = now
	task.CompletedAt = now
	task.CompletionSource = "manual"

	trace := &store.HumanTrace{
		TraceID:     newID("trace", now),
		TraceType:   "completion",
		ProjectID:   task.ProjectID,
		TaskID:      task.TaskID,
		TriggerType: "manual_complete",
		Status:      "completed",
		StartedAt:   task.StartedAt,
		CompletedAt: now,
		Summary:     scrubShort("Task marked complete manually.", 240),
		NextAction:  "Choose the next open loop.",
		Confidence:  1,
		MetadataJSON: metadata(map[string]string{
			"privacy": "summaries_only",
		}),
	}
	if _, err := store.InsertHumanTrace(db, trace); err != nil {
		return nil, err
	}
	return task, nil
}

// ReentryContext returns the best available next-action card.
func ReentryContext(db *store.DB) (*ReentryCard, error) {
	now := time.Now().Unix()
	trace, err := store.GetLatestHumanTraceByType(db, "interruption")
	if err != nil {
		return nil, err
	}
	if trace != nil {
		return cardForTrace(db, trace, now)
	}

	task, err := store.GetActiveOpenTask(db)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return &ReentryCard{
			Reason:      "No active v2 task has been inferred yet.",
			NextAction:  "Mark a task or keep working until Sauron has enough evidence.",
			GeneratedAt: now,
		}, nil
	}
	var project *store.Project
	if task.ProjectID != 0 {
		project, _ = store.GetProjectByID(db, task.ProjectID)
	}
	return &ReentryCard{
		Project:     project,
		Task:        task,
		Reason:      "Current open loop from the v2 task model.",
		NextAction:  task.NextAction,
		Confidence:  task.Confidence,
		GeneratedAt: now,
	}, nil
}

// ExplainTrace returns a v2 trace with project, task, and evidence.
func ExplainTrace(db *store.DB, traceID string) (*TraceExplanation, error) {
	if traceID == "" || traceID == "latest" {
		traces, err := store.GetRecentHumanTraces(db, 1)
		if err != nil {
			return nil, err
		}
		if len(traces) == 0 {
			return &TraceExplanation{}, nil
		}
		traceID = traces[0].TraceID
	}
	trace, err := store.GetHumanTraceByTraceID(db, traceID)
	if err != nil {
		return nil, err
	}
	if trace == nil {
		return nil, fmt.Errorf("v2 trace not found: %s", traceID)
	}
	events, err := store.GetHumanTraceEvents(db, trace.TraceID)
	if err != nil {
		return nil, err
	}
	var project *store.Project
	if trace.ProjectID != 0 {
		project, _ = store.GetProjectByID(db, trace.ProjectID)
	}
	var task *store.OpenTask
	if trace.TaskID != "" {
		task, _ = store.GetOpenTaskByTaskID(db, trace.TaskID)
	}
	return &TraceExplanation{Trace: trace, Project: project, Task: task, Events: events}, nil
}

// RecentTraces returns recent v2 traces.
func RecentTraces(db *store.DB, limit int) ([]store.HumanTrace, error) {
	if limit <= 0 {
		limit = 10
	}
	return store.GetRecentHumanTraces(db, limit)
}

// RecordCommitOutcome writes a v2 completion trace for a git commit without touching intent_traces.
func RecordCommitOutcome(db *store.DB, repoDir, message, hash string, now int64) error {
	if now == 0 {
		now = time.Now().Unix()
	}
	name := filepath.Base(repoDir)
	project := &store.Project{
		ProjectKey:   "repo:" + slug(repoDir),
		Name:         name,
		Kind:         "repo",
		RootHint:     scrubShort(repoDir, 240),
		CreatedAt:    now,
		UpdatedAt:    now,
		MetadataJSON: metadata(map[string]string{"source": "git_commit"}),
	}
	projectID, err := store.UpsertProject(db, project)
	if err != nil {
		return err
	}

	task, _ := store.GetActiveOpenTask(db)
	taskID := ""
	startedAt := now - DefaultLookbackMinutes*60
	nextAction := "Pick the next open loop."
	if task != nil && (task.ProjectID == projectID || task.Status == "active" || task.Status == "paused") {
		taskID = task.TaskID
		startedAt = task.StartedAt
		nextAction = task.NextAction
		_ = store.UpdateOpenTaskStatus(db, task.TaskID, "completed", "git_commit", now, now)
	}

	trace := &store.HumanTrace{
		TraceID:     newID("trace", now),
		TraceType:   "completion",
		ProjectID:   projectID,
		TaskID:      taskID,
		TriggerType: "git_commit",
		Status:      "completed",
		StartedAt:   startedAt,
		CompletedAt: now,
		Summary:     scrubShort(fmt.Sprintf("Git commit in %s: %s", name, message), 420),
		NextAction:  nextAction,
		Confidence:  0.9,
		MetadataJSON: metadata(map[string]string{
			"commit":  hash,
			"privacy": "summaries_only",
		}),
	}
	if _, err := store.InsertHumanTrace(db, trace); err != nil {
		return err
	}

	diff := commitDiffSummary(repoDir)
	if diff != "" {
		_, _ = store.InsertHumanTraceEvent(db, &store.HumanTraceEvent{
			TraceID:     trace.TraceID,
			Ts:          now,
			EventType:   "diff_summary",
			SourceTable: "git",
			Summary:     scrubShort(diff, 720),
			ArtifactURI: scrubShort(repoDir, 240),
			Severity:    "info",
			MetadataJSON: metadata(map[string]string{
				"commit": hash,
			}),
		})
	}
	return nil
}

func createInterruptionTrace(db *store.DB, task *store.OpenTask, project *store.Project, activities []store.ActivityEntry, sessions []store.ContextSession, clipboards []store.ClipboardItem, screenshots []store.Screenshot, now int64, reason string) error {
	latest, _ := store.GetLatestHumanTraceByType(db, "interruption")
	if latest != nil && latest.TaskID == task.TaskID && now-latest.CompletedAt < 15*60 {
		return nil
	}

	trace := &store.HumanTrace{
		TraceID:     newID("trace", now),
		TraceType:   "interruption",
		ProjectID:   task.ProjectID,
		TaskID:      task.TaskID,
		TriggerType: "drift",
		Status:      "paused",
		StartedAt:   task.StartedAt,
		CompletedAt: now,
		Summary:     scrubShort(fmt.Sprintf("Likely interruption: %s. Prior task: %s", reason, task.Goal), 420),
		NextAction:  task.NextAction,
		Confidence:  maxFloat(task.Confidence, 0.65),
		MetadataJSON: metadata(map[string]string{
			"privacy": "summaries_only",
		}),
	}
	if project != nil {
		trace.ProjectID = project.ID
	}
	if _, err := store.InsertHumanTrace(db, trace); err != nil {
		return err
	}

	for _, e := range buildEvidenceEvents(trace.TraceID, activities, sessions, clipboards, screenshots, now) {
		_, _ = store.InsertHumanTraceEvent(db, &e)
	}
	return nil
}

func cardForTrace(db *store.DB, trace *store.HumanTrace, now int64) (*ReentryCard, error) {
	events, err := store.GetHumanTraceEvents(db, trace.TraceID)
	if err != nil {
		return nil, err
	}
	var project *store.Project
	if trace.ProjectID != 0 {
		project, _ = store.GetProjectByID(db, trace.ProjectID)
	}
	var task *store.OpenTask
	if trace.TaskID != "" {
		task, _ = store.GetOpenTaskByTaskID(db, trace.TaskID)
	}
	return &ReentryCard{
		Project:     project,
		Task:        task,
		Trace:       trace,
		Events:      events,
		Reason:      trace.Summary,
		NextAction:  trace.NextAction,
		Confidence:  trace.Confidence,
		GeneratedAt: now,
	}, nil
}

func evidenceWindow(db *store.DB, now int64) ([]store.ActivityEntry, []store.ContextSession, []store.ClipboardItem, []store.Screenshot) {
	start := now - DefaultLookbackMinutes*60
	activities, _ := store.GetActivityInRange(db, start, now)
	sessions, _ := store.GetSessionsInRange(db, start, now)
	clipboards, _ := store.GetClipboardInRange(db, start, now)
	screenshots, _ := store.GetScreenshotsInRange(db, start, now)
	return activities, sessions, clipboards, screenshots
}

func inferLoop(activities []store.ActivityEntry, sessions []store.ContextSession, clipboards []store.ClipboardItem, now int64) *inference {
	for i := len(activities) - 1; i >= 0; i-- {
		a := activities[i]
		if isDistraction(a.AppName, a.WindowTitle) || weakApp(a.AppName) {
			continue
		}
		title := scrubShort(a.WindowTitle, 120)
		name := projectName(a.AppName, title)
		conf := 0.55
		if title != "" && title != a.AppName {
			conf += 0.15
		}
		if len(sessions) > 0 {
			last := sessions[len(sessions)-1]
			if last.FocusScore >= 0.7 {
				conf += 0.1
			}
		}
		lastUseful := fmt.Sprintf("Last useful context: %s", summarizeActivity(a))
		if len(clipboards) > 0 {
			c := clipboards[len(clipboards)-1]
			if !isDistraction(c.SourceApp, c.WindowTitle) {
				lastUseful += fmt.Sprintf("; recent clipboard from %s", emptyDefault(c.SourceApp, "unknown app"))
			}
		}
		next := "Return to " + a.AppName + "."
		return &inference{
			projectKey:      "app:" + slug(a.AppName+" "+title),
			projectName:     name,
			projectKind:     "inferred",
			goal:            "Continue " + name,
			lastUsefulState: scrubShort(lastUseful, 360),
			nextAction:      scrubShort(next, 240),
			confidence:      clamp(conf, 0, 0.9),
			activity:        &a,
		}
	}
	return nil
}

func detectDrift(activities []store.ActivityEntry, now int64) (bool, string) {
	if len(activities) == 0 {
		return false, ""
	}
	latest := activities[len(activities)-1]
	if !isDistraction(latest.AppName, latest.WindowTitle) {
		return false, ""
	}
	driftStart := latest.StartedAt
	if latest.EndedAt > 0 && latest.DurationMs > 0 {
		driftStart = latest.EndedAt - latest.DurationMs/1000
	}
	if now-driftStart < DriftThresholdSeconds {
		return false, ""
	}
	for _, a := range activities {
		if a.StartedAt < driftStart {
			continue
		}
		if !isDistraction(a.AppName, a.WindowTitle) && !weakApp(a.AppName) {
			return false, ""
		}
	}
	return true, fmt.Sprintf("%s looked unrelated for %s", summarizeActivity(latest), duration(now-driftStart))
}

func currentActivityIsDistraction(activities []store.ActivityEntry, now int64) bool {
	if len(activities) == 0 {
		return false
	}
	a := activities[len(activities)-1]
	return isDistraction(a.AppName, a.WindowTitle)
}

func buildEvidenceEvents(traceID string, activities []store.ActivityEntry, sessions []store.ContextSession, clipboards []store.ClipboardItem, screenshots []store.Screenshot, now int64) []store.HumanTraceEvent {
	var out []store.HumanTraceEvent
	add := func(e store.HumanTraceEvent) {
		e.TraceID = traceID
		e.Summary = scrubShort(e.Summary, 360)
		e.WindowTitle = scrubShort(e.WindowTitle, 160)
		e.ArtifactURI = scrubShort(e.ArtifactURI, 240)
		if e.Severity == "" {
			e.Severity = "info"
		}
		out = append(out, e)
	}

	for _, a := range tailActivities(activities, 6) {
		eventType := "activity"
		severity := "info"
		if isDistraction(a.AppName, a.WindowTitle) {
			eventType = "drift"
			severity = "warn"
		}
		add(store.HumanTraceEvent{
			Ts:          a.StartedAt,
			EventType:   eventType,
			SourceTable: "activity_log",
			SourceID:    a.ID,
			Summary:     summarizeActivity(a),
			AppName:     a.AppName,
			WindowTitle: a.WindowTitle,
			Severity:    severity,
		})
	}
	for _, s := range tailSessions(sessions, 3) {
		add(store.HumanTraceEvent{
			Ts:          s.StartedAt,
			EventType:   "session",
			SourceTable: "context_sessions",
			SourceID:    s.ID,
			Summary:     fmt.Sprintf("%s session, %.0f%% focus, dominant app %s", s.SessionType, s.FocusScore*100, s.DominantApp),
			AppName:     s.DominantApp,
			Severity:    "info",
		})
	}
	for _, c := range tailClipboards(clipboards, 3) {
		add(store.HumanTraceEvent{
			Ts:          c.CapturedAt,
			EventType:   "clipboard_ref",
			SourceTable: "clipboard_history",
			SourceID:    c.ID,
			Summary:     fmt.Sprintf("Clipboard changed in %s (%d chars, content not copied into v2 trace)", emptyDefault(c.SourceApp, "unknown app"), len(c.Content)),
			AppName:     c.SourceApp,
			WindowTitle: c.WindowTitle,
			Severity:    "info",
		})
	}
	for _, s := range tailScreenshots(screenshots, 3) {
		add(store.HumanTraceEvent{
			Ts:          s.CapturedAt,
			EventType:   "screenshot_ref",
			SourceTable: "screenshots",
			SourceID:    s.ID,
			Summary:     fmt.Sprintf("Screenshot captured from %s", emptyDefault(s.SourceApp, "unknown app")),
			AppName:     s.SourceApp,
			WindowTitle: s.WindowTitle,
			ArtifactURI: s.FilePath,
			Severity:    "info",
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Ts < out[j].Ts
	})
	return out
}

func tailActivities(items []store.ActivityEntry, n int) []store.ActivityEntry {
	if len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

func tailSessions(items []store.ContextSession, n int) []store.ContextSession {
	if len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

func tailClipboards(items []store.ClipboardItem, n int) []store.ClipboardItem {
	if len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

func tailScreenshots(items []store.Screenshot, n int) []store.Screenshot {
	if len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}

func isDistraction(app, title string) bool {
	text := strings.ToLower(app + " " + title)
	signals := []string{
		"youtube", "youtu.be", "netflix", "hulu", "disney+", "prime video",
		"twitch", "tiktok", "instagram", "facebook", "reddit", "x.com",
		"twitter", "reels", "shorts", "spotify", "discord",
	}
	for _, s := range signals {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func weakApp(app string) bool {
	a := strings.ToLower(app)
	weak := []string{"finder", "system settings", "settings", "loginwindow", "control center"}
	for _, w := range weak {
		if a == w {
			return true
		}
	}
	return false
}

func summarizeActivity(a store.ActivityEntry) string {
	title := strings.TrimSpace(a.WindowTitle)
	if title == "" || title == a.AppName {
		return a.AppName
	}
	return fmt.Sprintf("%s - %s", a.AppName, title)
}

func projectName(app, title string) string {
	title = strings.TrimSpace(title)
	if title == "" || title == app {
		return app
	}
	if idx := strings.Index(title, " - "); idx > 0 {
		title = title[:idx]
	}
	if idx := strings.Index(title, " — "); idx > 0 {
		title = title[:idx]
	}
	return fmt.Sprintf("%s / %s", app, strings.TrimSpace(title))
}

func nextActionFromGoal(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "Return to the marked task."
	}
	return "Continue: " + scrubShort(goal, 160)
}

func normalizeTaskStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "active", "paused", "completed", "abandoned":
		return status
	case "complete", "done":
		return "completed"
	case "abandon":
		return "abandoned"
	default:
		return "active"
	}
}

func newID(prefix string, now int64) string {
	return fmt.Sprintf("%s_%d", prefix, now*1_000_000_000+time.Now().UnixNano()%1_000_000_000)
}

func metadata(v map[string]string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func scrubShort(s string, n int) string {
	s = strings.Join(strings.Fields(scrub.Scrub(s)), " ")
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "..."
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "unknown"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func emptyDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return strings.TrimSpace(s)
}

func duration(sec int64) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	if min < 60 {
		return fmt.Sprintf("%dm", min)
	}
	return fmt.Sprintf("%dh %dm", min/60, min%60)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func commitDiffSummary(repoDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "show", "--stat", "--name-status", "--format=", "--no-renames", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
