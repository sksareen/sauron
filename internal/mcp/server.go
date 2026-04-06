package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sksareen/sauron/internal/query"
	"github.com/sksareen/sauron/internal/store"
)

// Start launches a stdio MCP server that exposes sauron tools.
// It blocks until stdin is closed or an error occurs.
func Start() error {
	db, err := store.OpenReadOnly()
	if err != nil {
		return fmt.Errorf("mcp: %w", err)
	}
	defer db.Close()

	s := server.NewMCPServer("sauron", "0.1.0",
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

	return server.ServeStdio(s)
}
