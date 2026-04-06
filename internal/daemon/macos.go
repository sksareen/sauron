package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GetFrontmostApp returns the current frontmost application info.
// It gracefully handles missing Accessibility permissions.
func GetFrontmostApp() (appName, bundleID, windowTitle string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Get app name and bundle ID — works without Accessibility perms.
	script := `
tell application "System Events"
	set fa to first application process whose frontmost is true
	set appName to name of fa
	set bundleID to bundle identifier of fa
	return appName & "|" & bundleID
end tell`

	out, err := runOsascript(ctx, script)
	if err != nil {
		return "", "", "", fmt.Errorf("get frontmost app: %w", err)
	}

	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	appName = parts[0]
	if len(parts) > 1 {
		bundleID = parts[1]
	}

	// Window title requires Accessibility — try, but don't fail if it errors.
	winCtx, winCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer winCancel()

	winScript := fmt.Sprintf(`
tell application "System Events"
	tell process "%s"
		if exists (window 1) then
			return name of window 1
		end if
	end tell
end tell`, appName)

	winOut, winErr := runOsascript(winCtx, winScript)
	if winErr == nil {
		windowTitle = strings.TrimSpace(winOut)
	}

	return appName, bundleID, windowTitle, nil
}

// GetClipboardContent returns the current clipboard text content.
func GetClipboardContent() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pbpaste")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pbpaste: %w", err)
	}
	return out.String(), nil
}

// WatchScreenshotsDir polls screenshot directories every 5 seconds for new files.
// It sends newly-found file paths on the returned channel.
// The channel is closed when the provided context is cancelled.
func WatchScreenshotsDir(ctx context.Context) (<-chan string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	dirs := []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Pictures", "Screenshots"),
	}

	ch := make(chan string, 16)

	go func() {
		defer close(ch)

		seen := make(map[string]struct{})

		// Seed seen with existing files so we don't re-emit on startup.
		for _, dir := range dirs {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if !e.IsDir() && isImageFile(e.Name()) {
					seen[filepath.Join(dir, e.Name())] = struct{}{}
				}
			}
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-10 * time.Second)
				for _, dir := range dirs {
					entries, err := os.ReadDir(dir)
					if err != nil {
						continue
					}
					for _, e := range entries {
						if e.IsDir() || !isImageFile(e.Name()) {
							continue
						}
						path := filepath.Join(dir, e.Name())
						if _, already := seen[path]; already {
							continue
						}
						info, err := e.Info()
						if err != nil {
							continue
						}
						if info.ModTime().After(cutoff) {
							seen[path] = struct{}{}
							select {
							case ch <- path:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return ch, nil
}

// TakeScreenshot captures the current screen and saves it to ~/.sauron/screenshots/.
// Returns the file path on success. Uses macOS screencapture command.
func TakeScreenshot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	dir := filepath.Join(home, ".sauron", "screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir screenshots: %w", err)
	}

	filename := fmt.Sprintf("sauron_%d.png", time.Now().UnixMilli())
	path := filepath.Join(dir, filename)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// -x = no sound, -C = capture cursor, -t png
	cmd := exec.CommandContext(ctx, "screencapture", "-x", "-C", "-t", "png", path)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("screencapture: %w", err)
	}

	return path, nil
}

func isImageFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg")
}

func runOsascript(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("osascript: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}
