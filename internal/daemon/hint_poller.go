package daemon

import (
	"context"
	"log"
	"time"

	"github.com/sksareen/sauron/internal/hint"
	"github.com/sksareen/sauron/internal/store"
)

const (
	hintMergeTick  = 10 * time.Second
	hintDecayTick  = 30 * time.Second
	hintLabelTick  = 30 * time.Second
)

func runHintPoller(ctx context.Context, db *store.DB) {
	merger := hint.NewMerger(db)
	labeller := hint.NewLabeller(db)

	mergeTicker := time.NewTicker(hintMergeTick)
	decayTicker := time.NewTicker(hintDecayTick)
	labelTicker := time.NewTicker(hintLabelTick)
	defer mergeTicker.Stop()
	defer decayTicker.Stop()
	defer labelTicker.Stop()

	// Track the last ingested IDs to avoid re-processing.
	var lastActivityID, lastClipboardID, lastSessionID int64

	for {
		select {
		case <-ctx.Done():
			return

		case <-mergeTicker.C:
			now := time.Now().Unix()

			// Ingest new activity entries.
			activities, err := store.GetActivity(db, 0.05) // last ~3 min
			if err == nil {
				for _, a := range activities {
					if a.ID <= lastActivityID {
						continue
					}
					if err := merger.IngestActivity(a); err != nil {
						log.Printf("hint: ingest activity %d: %v", a.ID, err)
					}
					if a.ID > lastActivityID {
						lastActivityID = a.ID
					}
				}
			}

			// Ingest new clipboard items.
			clips, err := store.GetRecentClipboard(db, 5)
			if err == nil {
				for _, c := range clips {
					if c.ID <= lastClipboardID {
						continue
					}
					if err := merger.IngestClipboard(c); err != nil {
						log.Printf("hint: ingest clipboard %d: %v", c.ID, err)
					}
					if c.ID > lastClipboardID {
						lastClipboardID = c.ID
					}
				}
			}

			// Ingest new sessions.
			sessions, err := store.GetSessionsInRange(db, now-180, now)
			if err == nil {
				for _, s := range sessions {
					if s.ID <= lastSessionID {
						continue
					}
					if err := merger.IngestSession(s); err != nil {
						log.Printf("hint: ingest session %d: %v", s.ID, err)
					}
					if s.ID > lastSessionID {
						lastSessionID = s.ID
					}
				}
			}

		case <-decayTicker.C:
			now := time.Now().Unix()
			if err := merger.RunDecay(now); err != nil {
				log.Printf("hint: decay: %v", err)
			}

		case <-labelTicker.C:
			now := time.Now().Unix()
			if err := labeller.RunLabels(now); err != nil {
				log.Printf("hint: label: %v", err)
			}
		}
	}
}
