package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
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
	"github.com/sksareen/sauron/internal/reentry"
	"github.com/sksareen/sauron/internal/store"
)

var version = "0.1.0"

func main() {
	root := &cobra.Command{
		Use:           "sauron",
		Short:         "sauron — the eternal witness",
		Long:          "sauron is a passive context daemon for macOS that captures clipboard, activity, and screenshots for AI agents.",
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

	// ── v2 project re-entry traces ───────────────────────────────────────────

	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Manage v2 project re-entry tasks",
	}

	var taskProject, taskStatus string
	var taskJSON bool
	taskMarkCmd := &cobra.Command{
		Use:   "mark <goal>",
		Short: "Mark or correct the current v2 open task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			task, err := reentry.MarkTask(db, strings.Join(args, " "), taskStatus, taskProject)
			if err != nil {
				return err
			}
			if taskJSON {
				b, _ := json.MarshalIndent(task, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			fmt.Printf("task %s %s\n", task.TaskID, task.Status)
			fmt.Printf("goal: %s\n", task.Goal)
			fmt.Printf("next: %s\n", task.NextAction)
			return nil
		},
	}
	taskMarkCmd.Flags().StringVar(&taskProject, "project", "", "project name or hint")
	taskMarkCmd.Flags().StringVar(&taskStatus, "status", "active", "active, paused, completed, or abandoned")
	taskMarkCmd.Flags().BoolVar(&taskJSON, "json", false, "machine-readable JSON output")
	taskCmd.AddCommand(taskMarkCmd)

	var taskCompleteJSON bool
	taskCompleteCmd := &cobra.Command{
		Use:   "complete [task_id]",
		Short: "Mark a v2 task complete",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			taskID := ""
			if len(args) > 0 {
				taskID = args[0]
			}
			task, err := reentry.CompleteTask(db, taskID)
			if err != nil {
				return err
			}
			if taskCompleteJSON {
				b, _ := json.MarshalIndent(task, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			fmt.Printf("completed %s\n", task.TaskID)
			return nil
		},
	}
	taskCompleteCmd.Flags().BoolVar(&taskCompleteJSON, "json", false, "machine-readable JSON output")
	taskCmd.AddCommand(taskCompleteCmd)
	root.AddCommand(taskCmd)

	var reentryJSON, reentryMD bool
	reentryCmd := &cobra.Command{
		Use:   "reentry",
		Short: "Show the v2 return-to-task card",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			if _, err := reentry.Evaluate(db, time.Now().Unix()); err != nil {
				return err
			}
			card, err := reentry.ReentryContext(db)
			if err != nil {
				return err
			}
			fmt.Println(formatReentryCard(card, formatFlag(reentryJSON, reentryMD)))
			return nil
		},
	}
	reentryCmd.Flags().BoolVar(&reentryJSON, "json", false, "machine-readable JSON output")
	reentryCmd.Flags().BoolVar(&reentryMD, "md", false, "markdown output")
	root.AddCommand(reentryCmd)

	var tracesV2JSON, tracesV2MD bool
	var tracesV2Limit int
	tracesV2Cmd := &cobra.Command{
		Use:   "traces-v2",
		Short: "List recent v2 human traces",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()

			traces, err := reentry.RecentTraces(db, tracesV2Limit)
			if err != nil {
				return err
			}
			fmt.Println(formatHumanTraces(traces, formatFlag(tracesV2JSON, tracesV2MD)))
			return nil
		},
	}
	tracesV2Cmd.Flags().BoolVar(&tracesV2JSON, "json", false, "machine-readable JSON output")
	tracesV2Cmd.Flags().BoolVar(&tracesV2MD, "md", false, "markdown output")
	tracesV2Cmd.Flags().IntVar(&tracesV2Limit, "limit", 10, "number of v2 traces to show")
	root.AddCommand(tracesV2Cmd)

	// ── capture (screenshot → annotate → route) ────────────────────────────────

	var captureRegister, captureSourceApp, captureBundleID string
	var captureInstall, captureStart, captureStop bool
	captureCmd := &cobra.Command{
		Use:   "capture",
		Short: "Interactive screenshot capture with annotation (Wispr Flow for screenshots)",
		Long:  "Take a screenshot, annotate it, and route it to your active Claude Code session.\nRun 'sauron capture --install' to set up the background app and global hotkey (Ctrl+Shift+S).",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, _ := os.UserHomeDir()
			appPath := filepath.Join(home, ".sauron", "SauronCapture.app")
			buildScript := filepath.Join(home, "coding", "sauron", "capture", "build.sh")

			if captureRegister != "" {
				// Register a screenshot file into the DB
				db, err := store.Open()
				if err != nil {
					return err
				}
				defer db.Close()

				sourceApp := captureSourceApp
				if sourceApp == "" {
					sourceApp = "SauronCapture"
				}
				bundleID := captureBundleID

				ts := time.Now().Unix()
				if err := store.InsertScreenshot(db, captureRegister, sourceApp, bundleID, "", ts); err != nil {
					return fmt.Errorf("register screenshot: %w", err)
				}
				fmt.Printf("registered: %s\n", captureRegister)
				return nil
			}

			if captureInstall {
				// Build the app
				fmt.Println("Building SauronCapture...")
				build := exec.Command("bash", buildScript)
				build.Stdout = os.Stdout
				build.Stderr = os.Stderr
				if err := build.Run(); err != nil {
					return fmt.Errorf("build failed: %w", err)
				}

				// Create LaunchAgent for auto-start
				plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.sauron.capture.plist")
				plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sauron.capture</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s/Contents/MacOS/SauronCapture</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>`, appPath)
				if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
					return fmt.Errorf("write LaunchAgent: %w", err)
				}
				fmt.Printf("LaunchAgent: %s\n", plistPath)
				fmt.Println("SauronCapture installed. Starting...")

				// Start it now
				exec.Command("open", appPath).Run()
				fmt.Println("Running. Global hotkey: Ctrl+Shift+S")
				return nil
			}

			if captureStop {
				exec.Command("pkill", "-f", "SauronCapture").Run()
				fmt.Println("SauronCapture stopped.")
				return nil
			}

			if captureStart {
				exec.Command("open", appPath).Run()
				fmt.Println("SauronCapture started. Global hotkey: Ctrl+Shift+S")
				return nil
			}

			// Default: one-shot capture (build if needed, then open)
			if _, err := os.Stat(appPath); os.IsNotExist(err) {
				fmt.Println("First run — building SauronCapture...")
				build := exec.Command("bash", buildScript)
				build.Stdout = os.Stdout
				build.Stderr = os.Stderr
				if err := build.Run(); err != nil {
					return fmt.Errorf("build failed: %w", err)
				}
			}
			return exec.Command("open", appPath).Run()
		},
	}
	captureCmd.Flags().StringVar(&captureRegister, "register", "", "register a screenshot file path in the DB")
	captureCmd.Flags().StringVar(&captureSourceApp, "source-app", "", "source app name (used with --register)")
	captureCmd.Flags().StringVar(&captureBundleID, "bundle-id", "", "source bundle ID (used with --register)")
	captureCmd.Flags().BoolVar(&captureInstall, "install", false, "build and install SauronCapture with auto-start")
	captureCmd.Flags().BoolVar(&captureStart, "start", false, "start SauronCapture")
	captureCmd.Flags().BoolVar(&captureStop, "stop", false, "stop SauronCapture")
	root.AddCommand(captureCmd)

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

	var (
		expLogTaskIntent    string
		expLogApproach      string
		expLogOutcome       string
		expLogToolsUsed     string
		expLogFailurePoints string
		expLogResolution    string
		expLogTags          string
	)
	expLogCmd := &cobra.Command{
		Use:   "log",
		Short: "Log a completed experience to the agent graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			if expLogTaskIntent == "" {
				return fmt.Errorf("--task-intent is required")
			}
			if expLogApproach == "" {
				expLogApproach = expLogTaskIntent
			}
			if expLogOutcome == "" {
				expLogOutcome = "partial"
			}
			if expLogOutcome != "success" && expLogOutcome != "failure" && expLogOutcome != "partial" {
				return fmt.Errorf("--outcome must be success, failure, or partial")
			}

			db, err := store.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			rec := &store.ExperienceRecord{
				TaskIntent: expLogTaskIntent,
				Approach:   expLogApproach,
				Outcome:    expLogOutcome,
				Resolution: expLogResolution,
				Source:     "cli",
			}
			if expLogToolsUsed != "" {
				rec.ToolsUsed = strings.Split(expLogToolsUsed, ",")
				for i, t := range rec.ToolsUsed {
					rec.ToolsUsed[i] = strings.TrimSpace(t)
				}
			}
			if expLogFailurePoints != "" {
				rec.FailurePoints = strings.Split(expLogFailurePoints, ",")
				for i, t := range rec.FailurePoints {
					rec.FailurePoints[i] = strings.TrimSpace(t)
				}
			}
			if expLogTags != "" {
				rec.Tags = strings.Split(expLogTags, ",")
				for i, t := range rec.Tags {
					rec.Tags[i] = strings.TrimSpace(t)
				}
			}

			if vec, err := embed.GetEmbedding(rec.TaskIntent + "\n" + rec.Approach); err == nil && vec != nil {
				rec.Embedding = embed.VectorToBytes(vec)
			}

			id, err := store.InsertExperience(db, rec)
			if err != nil {
				return err
			}
			count, _ := store.GetExperienceCount(db)
			fmt.Printf("logged experience id=%d  total=%d\n", id, count)
			return nil
		},
	}
	expLogCmd.Flags().StringVar(&expLogTaskIntent, "task-intent", "", "what was the task (required)")
	expLogCmd.Flags().StringVar(&expLogApproach, "approach", "", "what approach was taken")
	expLogCmd.Flags().StringVar(&expLogOutcome, "outcome", "partial", "success, failure, or partial")
	expLogCmd.Flags().StringVar(&expLogToolsUsed, "tools", "", "comma-separated tools used")
	expLogCmd.Flags().StringVar(&expLogFailurePoints, "failures", "", "comma-separated failure points")
	expLogCmd.Flags().StringVar(&expLogResolution, "resolution", "", "how failures were resolved")
	expLogCmd.Flags().StringVar(&expLogTags, "tags", "", "comma-separated tags")
	expCmd.AddCommand(expLogCmd)

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

	// ── hints ──────────────────────────────────────────────────────────────────

	var hintsJSON bool
	var hintsLimit int
	hintsCmd := &cobra.Command{
		Use:   "hints",
		Short: "Show active Human Intention Vectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenReadOnly()
			if err != nil {
				return err
			}
			defer db.Close()
			hints, err := query.GetHints(db, hintsLimit)
			if err != nil {
				return err
			}
			fmt.Println(query.FormatHints(hints, formatFlag(hintsJSON, false)))
			return nil
		},
	}
	hintsCmd.Flags().BoolVar(&hintsJSON, "json", false, "JSON output")
	hintsCmd.Flags().IntVar(&hintsLimit, "limit", 5, "max hints to show")
	root.AddCommand(hintsCmd)

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

func formatReentryCard(card *reentry.ReentryCard, format string) string {
	if card == nil {
		return "no re-entry context available"
	}
	if format == "json" {
		b, _ := json.MarshalIndent(card, "", "  ")
		return string(b)
	}
	var sb strings.Builder
	if format == "md" {
		sb.WriteString("## Re-entry\n\n")
	}
	if card.Task == nil {
		sb.WriteString(card.Reason)
		sb.WriteString("\n")
		sb.WriteString("next: ")
		sb.WriteString(card.NextAction)
		return strings.TrimRight(sb.String(), "\n")
	}
	if card.Project != nil {
		sb.WriteString(fmt.Sprintf("project: %s\n", card.Project.Name))
	}
	sb.WriteString(fmt.Sprintf("task:    %s\n", card.Task.Goal))
	sb.WriteString(fmt.Sprintf("status:  %s (%.0f%% confidence)\n", card.Task.Status, card.Confidence*100))
	if card.Reason != "" {
		sb.WriteString(fmt.Sprintf("reason:  %s\n", card.Reason))
	}
	if card.Task.LastUsefulState != "" {
		sb.WriteString(fmt.Sprintf("left:    %s\n", card.Task.LastUsefulState))
	}
	if card.NextAction != "" {
		sb.WriteString(fmt.Sprintf("next:    %s\n", card.NextAction))
	}
	if len(card.Events) > 0 {
		sb.WriteString("evidence:\n")
		for _, e := range card.Events {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", time.Unix(e.Ts, 0).Format("15:04:05"), e.Summary))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatHumanTraces(traces []store.HumanTrace, format string) string {
	if format == "json" {
		b, _ := json.MarshalIndent(traces, "", "  ")
		return string(b)
	}
	if len(traces) == 0 {
		if format == "md" {
			return "## V2 Human Traces\n\n_No v2 traces recorded._"
		}
		return "no v2 traces recorded yet"
	}
	var sb strings.Builder
	if format == "md" {
		sb.WriteString(fmt.Sprintf("## V2 Human Traces (%d)\n\n", len(traces)))
	} else {
		sb.WriteString(fmt.Sprintf("%d v2 trace(s)\n\n", len(traces)))
	}
	for _, t := range traces {
		ts := time.Unix(t.CompletedAt, 0).Format("2006-01-02 15:04")
		if format == "md" {
			sb.WriteString(fmt.Sprintf("### %s - %s\n\n", ts, t.TraceType))
			sb.WriteString(fmt.Sprintf("- **Status**: %s\n", t.Status))
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", t.Summary))
			if t.NextAction != "" {
				sb.WriteString(fmt.Sprintf("- **Next**: %s\n", t.NextAction))
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString(fmt.Sprintf("%s  %s  %s\n", ts, t.TraceType, t.Status))
			sb.WriteString(fmt.Sprintf("    %s\n", t.Summary))
			if t.NextAction != "" {
				sb.WriteString(fmt.Sprintf("    next: %s\n", t.NextAction))
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
