package daemon

import (
	"log"
	"sync"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// Screenshotter takes screenshots on events with a minimum cooldown.
type Screenshotter struct {
	db       *store.DB
	mu       sync.Mutex
	lastShot time.Time
	cooldown time.Duration
}

// NewScreenshotter creates a screenshotter with a minimum cooldown between captures.
func NewScreenshotter(db *store.DB, cooldown time.Duration) *Screenshotter {
	return &Screenshotter{
		db:       db,
		cooldown: cooldown,
	}
}

// Capture takes a screenshot if enough time has passed since the last one.
// It's safe to call from multiple goroutines.
// trigger describes why this screenshot was taken (e.g. "app_switch", "clipboard").
func (s *Screenshotter) Capture(trigger string) {
	s.mu.Lock()
	if time.Since(s.lastShot) < s.cooldown {
		s.mu.Unlock()
		return
	}
	s.lastShot = time.Now()
	s.mu.Unlock()

	// Take screenshot outside the lock.
	path, err := TakeScreenshot()
	if err != nil {
		log.Printf("screenshot (%s): capture failed: %v", trigger, err)
		return
	}

	appName, bundleID, windowTitle, _ := GetFrontmostApp()
	ts := time.Now().Unix()
	if err := store.InsertScreenshot(s.db, path, appName, bundleID, windowTitle, ts); err != nil {
		log.Printf("screenshot (%s): insert failed: %v", trigger, err)
	} else {
		log.Printf("screenshot (%s): %s [%s]", trigger, path, appName)
	}
}
