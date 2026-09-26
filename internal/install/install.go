package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sksareen/sauron/internal/daemon"
)

// plistContent is a format string: args are (executablePath, logPath, logPath).
const plistContent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sauron.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>`

const claudeMDHint = "\n<!-- sauron -->\nYou have access to sauron tools for real-time context and agent memory:\n- Watcher: sauron_context, sauron_clipboard, sauron_activity, sauron_search, sauron_recall, sauron_timeline\n- Experience graph: sauron_check_experience (search past experiences before starting tasks), sauron_log_experience (record completed tasks), sauron_experience_stats\nCheck sauron_context at session start. Check sauron_check_experience before non-trivial tasks. Log experiences after completing tasks.\nSauron does not capture the screen and does not need Accessibility permission.\n<!-- /sauron -->"

// Install sets up the LaunchAgent and MCP registration.
func Install() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home dir: %w", err)
	}

	// Drop any previously installed screen-capture agent so it cannot prompt.
	removeCaptureAgent(home)

	// 1. Write LaunchAgent plist.
	if err := writePlist(home); err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}
	if err := linkValinorBin(exePath); err != nil {
		return err
	}

	// 2. Register MCP in ~/.claude.json.
	if err := registerMCP(home); err != nil {
		return err
	}

	// 3. Append hint to ~/.claude/CLAUDE.md.
	if err := appendClaudeMDHint(home); err != nil {
		return err
	}

	fmt.Println("sauron: installed successfully")
	fmt.Println("  LaunchAgent: ~/Library/LaunchAgents/com.sauron.daemon.plist")
	fmt.Println("  MCP:         registered in ~/.claude.json")
	fmt.Println("  CLAUDE.md:   hint appended")
	fmt.Println("  Valinor:     ~/go/bin/sauron")
	fmt.Println()
	fmt.Println("Run 'sauron start' to start the daemon now.")
	fmt.Println("Or log out and back in for the LaunchAgent to pick it up automatically.")
	return nil
}

// LaunchFromApp is the double-click entry for Sauron.app.
func LaunchFromApp() error {
	if err := Install(); err != nil {
		return err
	}
	if err := daemon.Start(); err != nil && !strings.Contains(err.Error(), "already running") {
		return err
	}
	msg := "Sauron is running in the background.\n\nValinor finds it at ~/go/bin/sauron.\nNo accessibility or screen recording access is required."
	fmt.Println(msg)
	_ = exec.Command("osascript", "-e", `display dialog "Sauron is running in the background.\n\nValinor finds it at ~/go/bin/sauron.\nNo accessibility or screen recording access is required." buttons {"OK"} default button "OK" with title "Sauron"`).Run()
	return nil
}

// Uninstall reverses all installation steps.
func Uninstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home dir: %w", err)
	}

	errs := []string{}

	removeCaptureAgent(home)
	if exe, err := os.Executable(); err == nil {
		removeValinorLink(exe)
	}

	// Remove plist.
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.sauron.daemon.plist")
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove plist: %v", err))
	} else {
		fmt.Println("removed:", plistPath)
	}

	// Remove MCP from ~/.claude.json.
	if err := deregisterMCP(home); err != nil {
		errs = append(errs, fmt.Sprintf("deregister mcp: %v", err))
	} else {
		fmt.Println("removed sauron from ~/.claude.json")
	}

	// Remove CLAUDE.md hint.
	if err := removeClaudeMDHint(home); err != nil {
		errs = append(errs, fmt.Sprintf("remove claude.md hint: %v", err))
	} else {
		fmt.Println("removed sauron hint from ~/.claude/CLAUDE.md")
	}

	if len(errs) > 0 {
		return fmt.Errorf("uninstall errors: %s", strings.Join(errs, "; "))
	}

	fmt.Println("sauron: uninstalled")
	return nil
}

// --- helpers ---

func writePlist(home string) error {
	logPath := filepath.Join(home, ".sauron", "daemon.log")
	if err := os.MkdirAll(filepath.Join(home, ".sauron"), 0755); err != nil {
		return fmt.Errorf("creating .sauron dir: %w", err)
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}

	// Use the actual executable path so it works on both Intel (/usr/local/bin)
	// and Apple Silicon (/opt/homebrew/bin) installs.
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	content := fmt.Sprintf(plistContent, exePath, logPath, logPath)
	plistPath := filepath.Join(plistDir, "com.sauron.daemon.plist")
	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}
	return nil
}

func registerMCP(home string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	claudeJSONPath := filepath.Join(home, ".claude.json")

	var config map[string]any

	data, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading ~/.claude.json: %w", err)
		}
		// File doesn't exist — start fresh.
		config = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			// Malformed JSON — back it up and start fresh.
			backupPath := claudeJSONPath + ".bak"
			_ = os.WriteFile(backupPath, data, 0644)
			fmt.Printf("warning: ~/.claude.json was malformed; backed up to %s\n", backupPath)
			config = make(map[string]any)
		}
	}

	// Ensure mcpServers key exists.
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
		config["mcpServers"] = mcpServers
	}

	mcpServers["sauron"] = map[string]any{
		"type":    "stdio",
		"command": exePath,
		"args":    []string{"mcp"},
	}

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling ~/.claude.json: %w", err)
	}
	if err := os.WriteFile(claudeJSONPath, out, 0600); err != nil {
		return fmt.Errorf("writing ~/.claude.json: %w", err)
	}
	return nil
}

func deregisterMCP(home string) error {
	claudeJSONPath := filepath.Join(home, ".claude.json")

	data, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to do
		}
		return fmt.Errorf("reading ~/.claude.json: %w", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil // malformed, can't help
	}

	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}
	delete(mcpServers, "sauron")

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling ~/.claude.json: %w", err)
	}
	return os.WriteFile(claudeJSONPath, out, 0600)
}

func appendClaudeMDHint(home string) error {
	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")

	// Read existing content if any.
	data, err := os.ReadFile(claudeMDPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading CLAUDE.md: %w", err)
	}

	content := string(data)

	// Refresh an older hint that still advertised screen capture.
	if strings.Contains(content, "<!-- sauron -->") {
		if strings.Contains(content, "sauron_screenshots") || strings.Contains(content, "SauronCapture") {
			cleaned, ok := stripClaudeHint(content)
			if !ok {
				return nil
			}
			if err := os.WriteFile(claudeMDPath, []byte(cleaned), 0644); err != nil {
				return fmt.Errorf("rewriting CLAUDE.md: %w", err)
			}
			content = cleaned
		} else {
			return nil
		}
	}

	f, err := os.OpenFile(claudeMDPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening CLAUDE.md: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(claudeMDHint)
	return err
}

func removeClaudeMDHint(home string) error {
	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")

	data, err := os.ReadFile(claudeMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading CLAUDE.md: %w", err)
	}

	content := string(data)
	cleaned, ok := stripClaudeHint(content)
	if !ok {
		return nil
	}
	return os.WriteFile(claudeMDPath, []byte(cleaned), 0644)
}

func stripClaudeHint(content string) (string, bool) {
	if !strings.Contains(content, "<!-- sauron -->") {
		return content, false
	}
	start := strings.Index(content, "\n<!-- sauron -->")
	if start == -1 {
		start = strings.Index(content, "<!-- sauron -->")
	}
	end := strings.Index(content, "<!-- /sauron -->")
	if end == -1 {
		return content, false
	}
	end += len("<!-- /sauron -->")
	return content[:start] + content[end:], true
}

func linkValinorBin(exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(home, "go", "bin", "sauron")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("creating ~/go/bin: %w", err)
	}
	if same, _ := sameFile(dest, exePath); same {
		fmt.Println("  Valinor bin: " + dest)
		return nil
	}
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replacing %s: %w", dest, err)
	}
	if err := os.Symlink(exePath, dest); err != nil {
		return fmt.Errorf("linking %s: %w", dest, err)
	}
	return nil
}

func removeValinorLink(exePath string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dest := filepath.Join(home, "go", "bin", "sauron")
	info, err := os.Lstat(dest)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return
	}
	if same, _ := sameFile(dest, exePath); !same {
		return
	}
	_ = os.Remove(dest)
}

func sameFile(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return false, fmt.Errorf("resolve")
	}
	return ra == rb, nil
}

func removeCaptureAgent(home string) {
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.sauron.capture.plist")
	uid := strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/com.sauron.capture").Run()
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	_ = os.Remove(plistPath)
	// Exact process name only. Does not match the sauron daemon.
	_ = exec.Command("pkill", "-x", "SauronCapture").Run()
}
