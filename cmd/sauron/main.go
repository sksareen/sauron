package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sksareen/sauron/internal/daemon"
	"github.com/sksareen/sauron/internal/embed"
	"github.com/sksareen/sauron/internal/install"
	mcpserver "github.com/sksareen/sauron/internal/mcp"
	"github.com/sksareen/sauron/internal/query"
	"github.com/sksareen/sauron/internal/store"
)

var version = "0.2.0"

func main() {
	if launchedFromAppBundle() {
		if err := install.LaunchFromApp(); err != nil {
			fmt.Fprintln(os.Stderr, "sauron:", err)
			os.Exit(1)
		}
		return
	}

	root := &cobra.Command{
		Use:           "sauron",
		Short:         "sauron — the eternal witness",
		Long:          "sauron is a passive context daemon for macOS that records clipboard, apps, and git activity for Valinor and other agents. It does not capture the screen.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// ── daemon lifecycle ──────────────────────────────────────────────────────

	root.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the background capture daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Start()
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Stop()
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Daemon status and capture stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Status()
		},
	})

	// ── hidden internal command called by launchd ─────────────────────────────

	daemonCmd := &cobra.Command{
		Use:    "daemon",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.RunDaemon()
		},
	}
	root.AddCommand(daemonCmd)

	// ── query commands ────────────────────────────────────────────────────────

	var contextJSON, contextMD, contextBrief bool
	contextCmd := &cobra.Command{
		Use:   "context",
		Short: "What you're working on right now",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			summary, err := query.GetContext(db)
			if err != nil {
				return err
			}

			format := formatFlag(contextJSON, contextMD)
			if contextBrief {
				// One-paragraph brief version.
				if contextJSON {
					fmt.Println(query.FormatContext(summary, "json"))
				} else {
					fmt.Printf("%s | focus %.0f%% | %s\n",
						summary.SessionType,
						summary.FocusScore*100,
						summary.DominantApp)
				}
				return nil
			}
			fmt.Println(query.FormatContext(summary, format))
			return nil
		},
	}
	contextCmd.Flags().BoolVar(&contextJSON, "json", false, "machine-readable JSON output")
	contextCmd.Flags().BoolVar(&contextMD, "md", false, "markdown output for prompt injection")
	contextCmd.Flags().BoolVar(&contextBrief, "brief", false, "one-line summary")
	root.AddCommand(contextCmd)

	var clipboardJSON, clipboardMD bool
	clipboardCmd := &cobra.Command{
		Use:   "clipboard [n]",
		Short: "Last n clipboard items (default: 10)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := 10
			if len(args) > 0 {
				v, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("n must be an integer, got %q", args[0])
				}
				n = v
			}

			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			items, err := query.GetClipboard(db, n)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatClipboard(items, formatFlag(clipboardJSON, clipboardMD)))
			return nil
		},
	}
	clipboardCmd.Flags().BoolVar(&clipboardJSON, "json", false, "machine-readable JSON output")
	clipboardCmd.Flags().BoolVar(&clipboardMD, "md", false, "markdown output")
	root.AddCommand(clipboardCmd)

	var activityJSON, activityMD bool
	activityCmd := &cobra.Command{
		Use:   "activity [hours]",
		Short: "Last h hours of activity (default: 2)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hours := 2.0
			if len(args) > 0 {
				v, err := strconv.ParseFloat(args[0], 64)
				if err != nil {
					return fmt.Errorf("hours must be a number, got %q", args[0])
				}
				hours = v
			}

			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			summary, err := query.GetActivity(db, hours)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatActivity(summary, formatFlag(activityJSON, activityMD)))
			return nil
		},
	}
	activityCmd.Flags().BoolVar(&activityJSON, "json", false, "machine-readable JSON output")
	activityCmd.Flags().BoolVar(&activityMD, "md", false, "markdown output")
	root.AddCommand(activityCmd)

	var searchJSON, searchMD bool
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across everything captured",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := args[0]
			if len(args) > 1 {
				// Join multiple words into a single query.
				q = ""
				for i, a := range args {
					if i > 0 {
						q += " "
					}
					q += a
				}
			}

			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			results, err := query.Search(db, q)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatSearch(results, formatFlag(searchJSON, searchMD)))
			return nil
		},
	}
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "machine-readable JSON output")
	searchCmd.Flags().BoolVar(&searchMD, "md", false, "markdown output")
	root.AddCommand(searchCmd)

	// ── recall (semantic search over intent traces) ──────────────────────────

	var recallJSON, recallMD bool
	var recallHours float64
	recallCmd := &cobra.Command{
		Use:   "recall <query>",
		Short: "Semantic search over intent traces",
		Long:  "Find what you were doing when a particular outcome happened. Searches git commits, Claude sessions, and other detected outcomes.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.Join(args, " ")

			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			results, err := query.Recall(db, q, recallHours)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatRecall(results, formatFlag(recallJSON, recallMD)))
			return nil
		},
	}
	recallCmd.Flags().BoolVar(&recallJSON, "json", false, "machine-readable JSON output")
	recallCmd.Flags().BoolVar(&recallMD, "md", false, "markdown output")
	recallCmd.Flags().Float64Var(&recallHours, "hours", 168, "how far back to search (hours)")
	root.AddCommand(recallCmd)

	// ── timeline ─────────────────────────────────────────────────────────────

	var timelineJSON, timelineMD bool
	var timelineHours float64
	timelineCmd := &cobra.Command{
		Use:   "timeline",
		Short: "Show a fused timeline of recent activity + intent traces",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			now := time.Now().Unix()
			start := now - int64(timelineHours*3600)

			entries, err := query.GetTimeline(db, start, now)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatTimeline(entries, formatFlag(timelineJSON, timelineMD)))
			return nil
		},
	}
	timelineCmd.Flags().BoolVar(&timelineJSON, "json", false, "machine-readable JSON output")
	timelineCmd.Flags().BoolVar(&timelineMD, "md", false, "markdown output")
	timelineCmd.Flags().Float64Var(&timelineHours, "hours", 2, "how far back to show (hours)")
	root.AddCommand(timelineCmd)

	// ── traces ───────────────────────────────────────────────────────────────

	var tracesJSON, tracesMD bool
	var tracesLimit int
	tracesCmd := &cobra.Command{
		Use:   "traces",
		Short: "List recent intent traces",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			traces, err := query.GetTraces(db, tracesLimit)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatTraces(traces, formatFlag(tracesJSON, tracesMD)))
			return nil
		},
	}
	tracesCmd.Flags().BoolVar(&tracesJSON, "json", false, "machine-readable JSON output")
	tracesCmd.Flags().BoolVar(&tracesMD, "md", false, "markdown output")
	tracesCmd.Flags().IntVar(&tracesLimit, "limit", 10, "number of traces to show")
	root.AddCommand(tracesCmd)

	// ── Valinor open loops (hints + reentry) ────────────────────────────────────

	var hintsJSON bool
	hintsCmd := &cobra.Command{
		Use:   "hints",
		Short: "Manual threads Valinor can show and stop",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			hints, err := store.ListManualHints(db)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatHints(hints, formatFlag(hintsJSON, false)))
			return nil
		},
	}
	hintsCmd.Flags().BoolVar(&hintsJSON, "json", false, "JSON output")
	root.AddCommand(hintsCmd)

	var hintStartLabel string
	hintCmd := &cobra.Command{
		Use:   "hint",
		Short: "Start or stop a manual thread",
	}
	hintStartCmd := &cobra.Command{
		Use:   "start <label>",
		Short: "Start a manual thread Valinor can display",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := strings.Join(args, " ")
			if hintStartLabel != "" {
				label = hintStartLabel
			}
			id := fmt.Sprintf("hint-%d", time.Now().UnixNano())
			if !query.ValinorTaskID(id) {
				return fmt.Errorf("generated hint id is invalid")
			}
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := store.StartManualHint(db, id, label); err != nil {
				return err
			}
			fmt.Printf("{\"ok\":true,\"id\":%q}\n", id)
			return nil
		},
	}
	hintStartCmd.Flags().StringVar(&hintStartLabel, "label", "", "thread label")
	hintCmd.AddCommand(hintStartCmd)

	hintStopCmd := &cobra.Command{
		Use:   "stop <hint_id>",
		Short: "Stop a manual thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if !query.ValinorTaskID(id) {
				return fmt.Errorf("invalid hint id")
			}
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := store.StopManualHint(db, id); err != nil {
				return err
			}
			fmt.Println(`{"ok":true}`)
			return nil
		},
	}
	var hintStopJSON bool
	hintStopCmd.Flags().BoolVar(&hintStopJSON, "json", false, "JSON output")
	hintCmd.AddCommand(hintStopCmd)
	root.AddCommand(hintCmd)

	var reentryJSON bool
	reentryCmd := &cobra.Command{
		Use:   "reentry",
		Short: "Inferred unfinished thread for Valinor",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			re, err := query.BuildReentry(db, time.Now())
			if err != nil {
				return err
			}
			format := "human"
			if reentryJSON {
				format = "json"
			}
			fmt.Println(query.FormatReentry(re, format))
			return nil
		},
	}
	reentryCmd.Flags().BoolVar(&reentryJSON, "json", false, "JSON output")
	root.AddCommand(reentryCmd)

	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Update an inferred open thread",
	}
	var taskCompleteJSON bool
	taskCompleteCmd := &cobra.Command{
		Use:   "complete <task_id>",
		Short: "Mark an inferred thread done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if !query.ValinorTaskID(id) {
				return fmt.Errorf("invalid task id")
			}
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := store.CompleteTask(db, id); err != nil {
				return err
			}
			fmt.Println(`{"ok":true}`)
			_ = taskCompleteJSON
			return nil
		},
	}
	taskCompleteCmd.Flags().BoolVar(&taskCompleteJSON, "json", false, "JSON output")
	taskCmd.AddCommand(taskCompleteCmd)
	root.AddCommand(taskCmd)

	// ── experience graph ──────────────────────────────────────────────────────

	expCmd := &cobra.Command{
		Use:     "experience",
		Short:   "Agent experience graph commands",
		Aliases: []string{"exp"},
	}

	var expSearchJSON bool
	var expSearchLimit int
	expSearchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Semantic search over agent experiences",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.Join(args, " ")
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			results, total, err := query.CheckExperience(db, q, "", expSearchLimit)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatCheckExperience(results, total, formatFlag(expSearchJSON, false)))
			return nil
		},
	}
	expSearchCmd.Flags().BoolVar(&expSearchJSON, "json", false, "JSON output")
	expSearchCmd.Flags().IntVar(&expSearchLimit, "limit", 5, "max results")
	expCmd.AddCommand(expSearchCmd)

	var expStatsJSON bool
	expStatsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Experience graph statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			total, success, failure, partial, err := store.GetExperienceStats(db)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatExperienceStats(total, success, failure, partial, formatFlag(expStatsJSON, false)))
			return nil
		},
	}
	expStatsCmd.Flags().BoolVar(&expStatsJSON, "json", false, "JSON output")
	expCmd.AddCommand(expStatsCmd)

	var expRecentJSON bool
	expRecentCmd := &cobra.Command{
		Use:   "recent [n]",
		Short: "Show recent experiences (default: 10)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n := 10
			if len(args) > 0 {
				v, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("n must be an integer, got %q", args[0])
				}
				n = v
			}
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			records, err := store.GetRecentExperiences(db, n)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatRecentExperiences(records, formatFlag(expRecentJSON, false)))
			return nil
		},
	}
	expRecentCmd.Flags().BoolVar(&expRecentJSON, "json", false, "JSON output")
	expCmd.AddCommand(expRecentCmd)

	root.AddCommand(expCmd)

	// ── install/uninstall ──────────────────────────────────────────────────────

	root.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install LaunchAgent and register MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return install.Install()
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Remove LaunchAgent and MCP registration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return install.Uninstall()
		},
	})

	// ── mcp stdio server ──────────────────────────────────────────────────────

	root.AddCommand(&cobra.Command{
		Use:   "mcp",
		Short: "Start stdio MCP server (used by Claude Code)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.Start()
		},
	})

	// ── migrate-agentgraph ────────────────────────────────────────────────────

	root.AddCommand(&cobra.Command{
		Use:   "migrate-agentgraph",
		Short: "Import experiences from ~/.agentgraph/experiences.db into Sauron",
		RunE: func(cmd *cobra.Command, args []string) error {
			return migrateAgentGraph()
		},
	})

	// ── version ────────────────────────────────────────────────────────────────

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("sauron", version)
		},
	})

	// ── execute ────────────────────────────────────────────────────────────────

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "sauron:", err)
		os.Exit(1)
	}
}

func migrateAgentGraph() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	srcPath := filepath.Join(home, ".agentgraph", "experiences.db")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("agentgraph database not found at %s", srcPath)
	}

	srcDB, err := sql.Open("sqlite", srcPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("opening agentgraph db: %w", err)
	}
	defer srcDB.Close()

	dstDB, err := store.Open()
	if err != nil {
		return fmt.Errorf("opening sauron db: %w", err)
	}
	defer dstDB.Close()

	rows, err := srcDB.Query(`
		SELECT task_intent, approach, COALESCE(tools_used,''), COALESCE(failure_points,''),
		       COALESCE(resolution,''), outcome, COALESCE(tags,''), COALESCE(source,''),
		       embedding, COALESCE(created_at,'')
		FROM experiences`)
	if err != nil {
		return fmt.Errorf("querying agentgraph: %w", err)
	}
	defer rows.Close()

	var imported, skipped int
	for rows.Next() {
		var taskIntent, approach, toolsStr, failStr, resolution, outcome, tagsStr, source, createdAt string
		var embBlob []byte
		if err := rows.Scan(&taskIntent, &approach, &toolsStr, &failStr,
			&resolution, &outcome, &tagsStr, &source, &embBlob, &createdAt); err != nil {
			fmt.Fprintf(os.Stderr, "skip row: %v\n", err)
			skipped++
			continue
		}

		rec := &store.ExperienceRecord{
			TaskIntent: taskIntent,
			Approach:   approach,
			Resolution: resolution,
			Outcome:    outcome,
			Source:     source,
		}

		if toolsStr != "" {
			json.Unmarshal([]byte(toolsStr), &rec.ToolsUsed)
		}
		if failStr != "" {
			json.Unmarshal([]byte(failStr), &rec.FailurePoints)
		}
		if tagsStr != "" {
			json.Unmarshal([]byte(tagsStr), &rec.Tags)
		}

		// Convert Float64 embeddings to Float32.
		if len(embBlob) > 0 && len(embBlob)%8 == 0 {
			n := len(embBlob) / 8
			f32 := make([]float32, n)
			for i := 0; i < n; i++ {
				bits := binary.LittleEndian.Uint64(embBlob[i*8:])
				f32[i] = float32(math.Float64frombits(bits))
			}
			rec.Embedding = embed.VectorToBytes(f32)
		}

		if _, err := store.InsertExperience(dstDB, rec); err != nil {
			fmt.Fprintf(os.Stderr, "skip insert: %v\n", err)
			skipped++
			continue
		}
		imported++
	}

	fmt.Printf("Migrated %d experiences from AgentGraph", imported)
	if skipped > 0 {
		fmt.Printf(" (%d skipped)", skipped)
	}
	fmt.Println()
	return rows.Err()
}

func launchedFromAppBundle() bool {
	exe, err := os.Executable()
	if err != nil || !strings.Contains(exe, ".app/Contents/MacOS/") {
		return false
	}
	for _, arg := range os.Args[1:] {
		// Older macOS passed -psn_ when opening an app bundle. Anything else
		// is a real CLI invocation of the binary inside the bundle.
		if !strings.HasPrefix(arg, "-psn_") {
			return false
		}
	}
	return true
}

// formatFlag returns "json", "md", or "human" based on flags.
func formatFlag(asJSON, asMD bool) string {
	if asJSON {
		return "json"
	}
	if asMD {
		return "md"
	}
	return "human"
}
