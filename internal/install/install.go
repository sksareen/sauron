package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

const claudeMDHint = "\n<!-- sauron -->\nYou have access to sauron tools for real-time context and agent memory:\n- Watcher: sauron_context, sauron_clipboard, sauron_activity, sauron_search, sauron_recall, sauron_timeline, sauron_screenshots\n- Experience graph: sauron_check_experience (search past experiences before starting tasks), sauron_log_experience (record completed tasks), sauron_experience_stats\nCheck sauron_context at session start. Check sauron_check_experience before non-trivial tasks. Log experiences after completing tasks.\n<!-- /sauron -->"

// Install sets up the LaunchAgent and MCP registration.
func Install() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home dir: %w", err)
	}

	// 1. Write LaunchAgent plist.
	if err := writePlist(home); err != nil {
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
	fmt.Println()
	fmt.Println("Run 'sauron start' to start the daemon now.")
	fmt.Println("Or log out and back in for the LaunchAgent to pick it up automatically.")
	return nil
}

// Uninstall reverses all installation steps.
func Uninstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home dir: %w", err)
	}

	errs := []string{}

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

	// Idempotent — don't add twice.
	if strings.Contains(content, "<!-- sauron -->") {
		return nil
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
	if !strings.Contains(content, "<!-- sauron -->") {
		return nil
	}

	// Remove the block.
	start := strings.Index(content, "\n<!-- sauron -->")
	if start == -1 {
		start = strings.Index(content, "<!-- sauron -->")
	}
	end := strings.Index(content, "<!-- /sauron -->")
	if end == -1 {
		return nil
	}
	end += len("<!-- /sauron -->")

	cleaned := content[:start] + content[end:]
	return os.WriteFile(claudeMDPath, []byte(cleaned), 0644)
}
