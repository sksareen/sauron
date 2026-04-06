package daemon

import (
	"context"
	"log"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// activityPoller tracks active application usage.
type activityPoller struct {
	db *store.DB
	ss *Screenshotter

	currentApp     string
	currentBundleID string
	currentTitle   string
	currentID      int64
	currentStart   int64

	// For session classification.
	recentApps    []string // most-recent first
	switchCount   int
	switchWindow  time.Time // start of 10-min switch window
	lastActivity  time.Time
	sessionStart  int64
	lastSessionAt time.Time
}

// runActivityPoller polls every 5 seconds and updates session every 30 seconds.
func runActivityPoller(ctx context.Context, db *store.DB, ss *Screenshotter) {
	p := &activityPoller{
		db:           db,
		ss:           ss,
		switchWindow: time.Now(),
		lastActivity: time.Now(),
		sessionStart: time.Now().Unix(),
		lastSessionAt: time.Now(),
	}

	activityTicker := time.NewTicker(5 * time.Second)
	sessionTicker := time.NewTicker(30 * time.Second)
	defer activityTicker.Stop()
	defer sessionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.endCurrent(time.Now().Unix())
			return
		case <-activityTicker.C:
			p.pollActivity()
		case <-sessionTicker.C:
			p.updateSession()
		}
	}
}

func (p *activityPoller) pollActivity() {
	appName, bundleID, windowTitle, err := GetFrontmostApp()
	if err != nil {
		return
	}
	if appName == "" {
		return
	}

	now := time.Now()
	nowUnix := now.Unix()

	// Reset switch count window every 10 minutes.
	if now.Sub(p.switchWindow) > 10*time.Minute {
		p.switchCount = 0
		p.switchWindow = now
	}

	if appName != p.currentApp {
		// App changed: close current entry, open new one.
		p.endCurrent(nowUnix)

		id, err := store.InsertActivity(p.db, appName, bundleID, windowTitle, nowUnix)
		if err != nil {
			log.Printf("activity: insert failed: %v", err)
		}
		p.currentApp = appName
		p.currentBundleID = bundleID
		p.currentTitle = windowTitle
		p.currentID = id
		p.currentStart = nowUnix

		p.switchCount++

		// Screenshot on app switch.
		if p.ss != nil {
			go p.ss.Capture("app_switch")
		}

		// Prepend to recent apps.
		p.recentApps = append([]string{appName}, p.recentApps...)
		if len(p.recentApps) > 50 {
			p.recentApps = p.recentApps[:50]
		}
	}

	p.lastActivity = now
}

func (p *activityPoller) endCurrent(nowUnix int64) {
	if p.currentID == 0 {
		return
	}
	if err := store.EndActivity(p.db, p.currentID, nowUnix); err != nil {
		log.Printf("activity: end failed: %v", err)
	}
	p.currentID = 0
}

func (p *activityPoller) updateSession() {
	now := time.Now()
	nowUnix := now.Unix()

	// Minutes in current app.
	sameAppMinutes := float64(nowUnix-p.currentStart) / 60.0

	// Idle detection.
	idleSeconds := now.Sub(p.lastActivity).Seconds()

	sessionType := ClassifySession(p.recentApps, sameAppMinutes, p.switchCount, idleSeconds)
	focusScore := FocusScore(p.switchCount)

	dominantApp := p.currentApp
	if len(p.recentApps) > 0 {
		dominantApp = p.recentApps[0]
	}

	if err := store.InsertSession(p.db, sessionType, focusScore, p.sessionStart, p.switchCount, dominantApp); err != nil {
		log.Printf("activity: insert session failed: %v", err)
	}

	// Roll the session window.
	p.sessionStart = nowUnix
	p.lastSessionAt = now
}
