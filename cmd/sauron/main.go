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
		Use:   "experience",
		Short: "Agent experience graph commands",
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
