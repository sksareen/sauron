package daemon

import (
	"context"
	"log"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// clipboardPoller watches the clipboard and inserts new entries into the DB.
type clipboardPoller struct {
	db       *store.DB
	ss       *Screenshotter
	lastSeen string
}

// runClipboardPoller polls the clipboard every second.
// It returns when ctx is cancelled.
func runClipboardPoller(ctx context.Context, db *store.DB, ss *Screenshotter) {
	p := &clipboardPoller{db: db, ss: ss}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *clipboardPoller) poll() {
	content, err := GetClipboardContent()
	if err != nil {
		// Non-fatal — clipboard may be empty or hold non-text data.
		return
	}

	if content == "" || content == p.lastSeen {
		return
	}

	p.lastSeen = content

	appName, bundleID, windowTitle, err := GetFrontmostApp()
	if err != nil {
		// Still insert, just without app context.
		log.Printf("clipboard: could not get frontmost app: %v", err)
	}

	if err := store.InsertClipboard(p.db, content, "text", appName, bundleID, windowTitle); err != nil {
		log.Printf("clipboard: insert failed: %v", err)
	}

	// Screenshot on clipboard change.
	if p.ss != nil {
		go p.ss.Capture("clipboard")
	}
}
