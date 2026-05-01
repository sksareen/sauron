package daemon

import (
	"context"
	"log"
	"time"

	"github.com/sksareen/sauron/internal/reentry"
	"github.com/sksareen/sauron/internal/store"
)

const reentryPollInterval = 30 * time.Second

// runReentryPoller maintains the additive v2 project/task/trace model.
func runReentryPoller(ctx context.Context, db *store.DB) {
	ticker := time.NewTicker(reentryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			loop, err := reentry.Evaluate(db, time.Now().Unix())
			if err != nil {
				log.Printf("reentry: evaluate failed: %v", err)
				continue
			}
			if loop != nil && loop.DriftDetected {
				log.Printf("reentry: drift detected: %s", loop.Reason)
			}
		}
	}
}
