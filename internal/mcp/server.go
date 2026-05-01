package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sksareen/sauron/internal/embed"
	"github.com/sksareen/sauron/internal/query"
	"github.com/sksareen/sauron/internal/reentry"
	"github.com/sksareen/sauron/internal/scrub"
	"github.com/sksareen/sauron/internal/store"
)

// Start launches a stdio MCP server that exposes sauron tools.
// It blocks until stdin is closed or an error occurs.
func Start() error {
	// Open read-write: experience tools need write access.
	db, err := store.Open()
	if err != nil {
		return fmt.Errorf("mcp: %w", err)
	}
	defer db.Close()

	s := server.NewMCPServer("sauron", "0.2.0",
		server.WithToolCapabilities(true),
	)

	// sauron_context
	s.AddTool(
		mcp.NewTool("sauron_context",
			mcp.WithDescription("Get a summary of what the user is currently working on: session type, focus score, dominant app, and recent clipboard."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			summary, err := query.GetContext(db)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatContext(summary, "human")), nil
		},
	)

	// sauron_clipboard
	s.AddTool(
		mcp.NewTool("sauron_clipboard",
			mcp.WithDescription("Get the most recent clipboard items captured from the user's Mac."),
			mcp.WithNumber("n",
				mcp.Description("Number of items to return (default: 10)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			n := req.GetInt("n", 10)
			if n <= 0 {
				n = 10
			}
			items, err := query.GetClipboard(db, n)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatClipboard(items, "human")), nil
		},
	)

	// sauron_activity
	s.AddTool(
		mcp.NewTool("sauron_activity",
			mcp.WithDescription("Get a summary of the user's app activity over the last N hours."),
			mcp.WithNumber("hours",
				mcp.Description("Number of hours to look back (default: 2)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			hours := req.GetFloat("hours", 2.0)
			if hours <= 0 {
				hours = 2.0
			}
			summary, err := query.GetActivity(db, hours)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatActivity(summary, "human")), nil
		},
	)

	// sauron_search
	s.AddTool(
		mcp.NewTool("sauron_search",
			mcp.WithDescription("Full-text search across all captured clipboard content."),
			mcp.WithString("query",
				mcp.Description("The search query"),
				mcp.Required(),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			q, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("query parameter required"), nil
			}
			results, err := query.Search(db, q)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatSearch(results, "human")), nil
		},
	)

	// sauron_screenshots
	s.AddTool(
		mcp.NewTool("sauron_screenshots",
			mcp.WithDescription("Get recent screenshots captured from the user's Mac. Returns file paths that can be read with the Read tool to view the images. Screenshots are taken automatically on app switches and clipboard changes."),
			mcp.WithNumber("n",
				mcp.Description("Number of screenshots to return (default: 5)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			n := req.GetInt("n", 5)
			if n <= 0 {
				n = 5
			}
			items, err := query.GetScreenshots(db, n)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatScreenshots(items, "human")), nil
		},
	)

	// sauron_recall — semantic search over intent traces
	s.AddTool(
		mcp.NewTool("sauron_recall",
			mcp.WithDescription("Semantic search over intent traces — find what the user was doing when a particular outcome happened. Searches git commits, Claude sessions, and other detected outcomes."),
			mcp.WithString("query",
				mcp.Description("Natural language query describing what you're looking for"),
				mcp.Required(),
			),
			mcp.WithNumber("hours",
				mcp.Description("How far back to search in hours (default: 168 = 1 week)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			q, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("query parameter required"), nil
			}
			hours := req.GetFloat("hours", 168)
			if hours <= 0 {
				hours = 168
			}
			results, err := query.Recall(db, q, hours)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatRecall(results, "human")), nil
		},
	)

	// sauron_timeline — fused timeline of all event types
	s.AddTool(
		mcp.NewTool("sauron_timeline",
			mcp.WithDescription("Get a fused timeline of activity, clipboard, sessions, and intent traces in a time window. Correlate human context with agent experiences."),
			mcp.WithNumber("start_time",
				mcp.Description("Start of window as unix timestamp"),
				mcp.Required(),
			),
			mcp.WithNumber("end_time",
				mcp.Description("End of window as unix timestamp"),
				mcp.Required(),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			startTime := req.GetInt("start_time", 0)
			endTime := req.GetInt("end_time", 0)
			if startTime == 0 || endTime == 0 {
				return mcp.NewToolResultError("start_time and end_time are required"), nil
			}
			entries, err := query.GetTimeline(db, int64(startTime), int64(endTime))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatTimeline(entries, "human")), nil
		},
	)

	// ── v2 project re-entry tools ───────────────────────────────────────────

	s.AddTool(
		mcp.NewTool("sauron_current_loop",
			mcp.WithDescription("Get the current v2 project/task open loop inferred from local activity. This reads the parallel v2 task model and does not modify legacy traces."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			loop, err := reentry.CurrentLoop(db)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(mcpJSON(loop)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("sauron_reentry_context",
			mcp.WithDescription("Get the best soft re-entry card: prior task, last useful state, evidence summaries, interruption reason, and next action."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if _, err := reentry.Evaluate(db, 0); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			card, err := reentry.ReentryContext(db)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(mcpJSON(card)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("sauron_mark_task",
			mcp.WithDescription("Create, update, complete, or correct a v2 open task. Writes only to the parallel v2 task/trace tables."),
			mcp.WithString("goal",
				mcp.Description("The task goal or correction. Required unless completing an existing task."),
			),
			mcp.WithString("status",
				mcp.Description("active, paused, completed, or abandoned (default: active)"),
			),
			mcp.WithString("task_id",
				mcp.Description("Optional v2 task_id to complete"),
			),
			mcp.WithString("project",
				mcp.Description("Optional project name or hint"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			goal, _ := req.RequireString("goal")
			status, _ := req.RequireString("status")
			taskID, _ := req.RequireString("task_id")
			project, _ := req.RequireString("project")
			if status == "completed" || status == "complete" || status == "done" {
				task, err := reentry.CompleteTask(db, taskID)
				if err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
				return mcp.NewToolResultText(mcpJSON(task)), nil
			}
			if strings.TrimSpace(goal) == "" {
				return mcp.NewToolResultError("goal is required unless status is completed"), nil
			}
			task, err := reentry.MarkTask(db, goal, status, project)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(mcpJSON(task)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("sauron_explain_trace",
			mcp.WithDescription("Explain a v2 human trace with linked evidence summaries. Pass trace_id or 'latest'."),
			mcp.WithString("trace_id",
				mcp.Description("The v2 trace_id to explain, or latest"),
				mcp.Required(),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			traceID, err := req.RequireString("trace_id")
			if err != nil {
				return mcp.NewToolResultError("trace_id parameter required"), nil
			}
			explained, err := reentry.ExplainTrace(db, traceID)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(mcpJSON(explained)), nil
		},
	)

	// ── experience graph tools ─────────────────────────────────────────────

	// sauron_check_experience — semantic search over agent experiences
	s.AddTool(
		mcp.NewTool("sauron_check_experience",
			mcp.WithDescription("Search the agent experience graph for relevant past experiences before starting a task. Returns approaches that worked, failures to avoid, and tools that helped."),
			mcp.WithString("task_description",
				mcp.Description("What you're about to do — be specific about the goal"),
				mcp.Required(),
			),
			mcp.WithString("context",
				mcp.Description("Optional context: tools, constraints, environment"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Max results to return (default: 5)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			taskDesc, err := req.RequireString("task_description")
			if err != nil {
				return mcp.NewToolResultError("task_description parameter required"), nil
			}
			ctxStr, _ := req.RequireString("context")
			limit := req.GetInt("limit", 5)
			if limit <= 0 {
				limit = 5
			}

			results, total, err := query.CheckExperience(db, taskDesc, ctxStr, limit)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatCheckExperience(results, total, "human")), nil
		},
	)

	// sauron_log_experience — record a completed task
	s.AddTool(
		mcp.NewTool("sauron_log_experience",
			mcp.WithDescription("Log a completed task to the agent experience graph so future agents can learn from it. Records what worked, what failed, and how issues were resolved."),
			mcp.WithString("task_intent",
				mcp.Description("What was the task? Be specific about the goal."),
				mcp.Required(),
			),
			mcp.WithString("approach",
				mcp.Description("What approach did you take? Include key decisions and steps."),
				mcp.Required(),
			),
			mcp.WithString("outcome",
				mcp.Description("Result: success, failure, or partial"),
				mcp.Required(),
			),
			mcp.WithString("tools_used",
				mcp.Description("Comma-separated list of tools/technologies used"),
			),
			mcp.WithString("failure_points",
				mcp.Description("Comma-separated list of things that went wrong"),
			),
			mcp.WithString("resolution",
				mcp.Description("How failures were fixed"),
			),
			mcp.WithString("tags",
				mcp.Description("Comma-separated tags for categorization"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			taskIntent, err := req.RequireString("task_intent")
			if err != nil {
				return mcp.NewToolResultError("task_intent required"), nil
			}
			approach, err := req.RequireString("approach")
			if err != nil {
				return mcp.NewToolResultError("approach required"), nil
			}
			outcome, err := req.RequireString("outcome")
			if err != nil {
				return mcp.NewToolResultError("outcome required"), nil
			}
			if outcome != "success" && outcome != "failure" && outcome != "partial" {
				return mcp.NewToolResultError("outcome must be success, failure, or partial"), nil
			}

			rec := &store.ExperienceRecord{
				TaskIntent: taskIntent,
				Approach:   approach,
				Outcome:    outcome,
				Source:     "live",
			}

			if tools, _ := req.RequireString("tools_used"); tools != "" {
				rec.ToolsUsed = splitCSV(tools)
			}
			if fails, _ := req.RequireString("failure_points"); fails != "" {
				rec.FailurePoints = splitCSV(fails)
			}
			if res, _ := req.RequireString("resolution"); res != "" {
				rec.Resolution = res
			}
			if tags, _ := req.RequireString("tags"); tags != "" {
				rec.Tags = splitCSV(tags)
			}

			// Scrub sensitive data.
			scrub.ScrubRecord(rec)

			// Generate embedding.
			embText := rec.TaskIntent + "\n" + rec.Approach
			if vec, err := embed.GetEmbedding(embText); err == nil && vec != nil {
				rec.Embedding = embed.VectorToBytes(vec)
			}

			id, err := store.InsertExperience(db, rec)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			count, _ := store.GetExperienceCount(db)
			return mcp.NewToolResultText(fmt.Sprintf("Experience logged (id: %d). Graph now has %d records.", id, count)), nil
		},
	)

	// sauron_experience_stats — graph statistics
	s.AddTool(
		mcp.NewTool("sauron_experience_stats",
			mcp.WithDescription("Get statistics about the agent experience graph: total records, success/failure/partial breakdown."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			total, success, failure, partial, err := store.GetExperienceStats(db)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatExperienceStats(total, success, failure, partial, "human")), nil
		},
	)

	// sauron_hints — active Human Intention Vectors
	s.AddTool(
		mcp.NewTool("sauron_hints",
			mcp.WithDescription("Get active Human Intention Vectors — LLM-inferred labels of what the user is working on right now, with weight, evidence, and confidence."),
			mcp.WithNumber("limit",
				mcp.Description("Max hints to return (default: 3)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			limit := req.GetInt("limit", 3)
			if limit <= 0 {
				limit = 3
			}
			hints, err := query.GetHints(db, limit)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(query.FormatHints(hints, "human")), nil
		},
	)

	return server.ServeStdio(s)
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func mcpJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
