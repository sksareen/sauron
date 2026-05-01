package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// PIDPath returns the path to the sauron PID file.
func PIDPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sauron", "sauron.pid")
}

// Start launches the daemon in the background.
// It forks the current process with the hidden "daemon" sub-command.
func Start() error {
	if pid, err := readPID(); err == nil {
		if isAlive(pid) {
			return fmt.Errorf("sauron: daemon already running (pid %d)", pid)
		}
		// Stale PID file — remove and continue.
		os.Remove(PIDPath())
	}

	// Re-exec self with "daemon" command to run in background.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find executable: %w", err)
	}

	// Ensure .sauron dir exists.
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(filepath.Join(home, ".sauron"), 0755); err != nil {
		return fmt.Errorf("creating .sauron dir: %w", err)
	}

	logPath := filepath.Join(home, ".sauron", "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	cmd := exec.Command(exe, "daemon")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting daemon process: %w", err)
	}

	logFile.Close()

	// Wait briefly for the daemon to write its PID file.
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := readPID(); err == nil {
			break
		}
	}

	fmt.Printf("sauron: daemon started (pid %d)\n", cmd.Process.Pid)
	fmt.Printf("sauron: log at %s\n", logPath)
	return nil
}

// Stop sends SIGTERM to the daemon process.
func Stop() error {
	pid, err := readPID()
	if err != nil {
		return fmt.Errorf("sauron: daemon not running")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(PIDPath())
		return fmt.Errorf("sauron: cannot find process %d", pid)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		os.Remove(PIDPath())
		return fmt.Errorf("sauron: failed to stop daemon: %w", err)
	}

	// Wait for it to die.
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isAlive(pid) {
			break
		}
	}

	os.Remove(PIDPath())
	fmt.Println("sauron: daemon stopped")
	return nil
}

// Status prints daemon status and capture stats.
func Status() error {
	pid, err := readPID()
	if err != nil || !isAlive(pid) {
		fmt.Println("sauron: not running")
		return nil
	}

	fmt.Printf("sauron: running (pid %d)\n", pid)

	db, err := store.OpenReadOnly()
	if err != nil {
		fmt.Printf("sauron: could not open database: %v\n", err)
		return nil
	}
	defer db.Close()

	var clipCount int
	db.QueryRow("SELECT COUNT(*) FROM clipboard_history").Scan(&clipCount)

	var activityCount int
	db.QueryRow("SELECT COUNT(*) FROM activity_log").Scan(&activityCount)

	var sessionCount int
	db.QueryRow("SELECT COUNT(*) FROM context_sessions").Scan(&sessionCount)

	var vercelCount int
	db.QueryRow("SELECT COUNT(*) FROM live_vercel_logs").Scan(&vercelCount)

	fmt.Printf("clipboard captures: %d\n", clipCount)
	fmt.Printf("activity entries:   %d\n", activityCount)
	fmt.Printf("sessions:           %d\n", sessionCount)
	fmt.Printf("live:vercel logs:   %d (watching %s)\n", vercelCount, VercelDeploymentURL())
	return nil
}

// RunDaemon is the actual daemon loop — called by the hidden "daemon" sub-command.
func RunDaemon() error {
	// Write PID file.
	if err := writePID(os.Getpid()); err != nil {
		return fmt.Errorf("writing pid file: %w", err)
	}
	defer os.Remove(PIDPath())

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("sauron daemon starting (pid %d)", os.Getpid())

	db, err := store.Open()
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		log.Println("sauron daemon shutting down")
		cancel()
	}()

	// Start screenshot watcher.
	screenshotCh, err := WatchScreenshotsDir(ctx)
	if err != nil {
		log.Printf("screenshot watcher error: %v", err)
	} else {
		go func() {
			for path := range screenshotCh {
				appName, bundleID, windowTitle, _ := GetFrontmostApp()
				ts := time.Now().Unix()
				if err := store.InsertScreenshot(db, path, appName, bundleID, windowTitle, ts); err != nil {
					log.Printf("screenshot insert: %v", err)
				} else {
					log.Printf("screenshot captured: %s", path)
				}
			}
		}()
	}

	// Shared screenshotter: 5-second cooldown between captures.
	ss := NewScreenshotter(db, 5*time.Second)

	// Start clipboard, activity, and intent pollers.
	go runClipboardPoller(ctx, db, ss)
	go runActivityPoller(ctx, db, ss)
	go runIntentPoller(ctx, db)
	go runReentryPoller(ctx, db)
	go runHintPoller(ctx, db)

	// Start live:vercel poller. Gate on the CLI being installed + authed so
	// we fail loudly on startup and don't silently swallow auth errors.
	vercelURL := VercelDeploymentURL()
	if err := EnsureVercelCLI(ctx); err != nil {
		log.Printf("vercel: disabled — %v", err)
	} else {
		go RunVercelPoller(ctx, db, vercelURL)
	}

	log.Println("sauron daemon running")
	<-ctx.Done()
	log.Println("sauron daemon stopped")
	return nil
}

// --- helpers ---

func writePID(pid int) error {
	if err := os.MkdirAll(filepath.Dir(PIDPath()), 0755); err != nil {
		return err
	}
	return os.WriteFile(PIDPath(), []byte(strconv.Itoa(pid)), 0644)
}

func readPID() (int, error) {
	data, err := os.ReadFile(PIDPath())
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func isAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence without sending a signal.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
