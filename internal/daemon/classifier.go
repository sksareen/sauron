package daemon

import "strings"

// SessionType constants.
const (
	SessionDeepFocus     = "deep_focus"
	SessionExploration   = "exploration"
	SessionCommunication = "communication"
	SessionCreative      = "creative"
	SessionAdmin         = "admin"
	SessionIdle          = "idle"
)

// communicationApps are apps that indicate communication activity.
var communicationApps = []string{
	"slack", "mail", "messages", "zoom", "microsoft teams", "teams",
	"discord", "telegram", "whatsapp", "skype", "facetime",
}

// creativeApps are apps that indicate creative work.
var creativeApps = []string{
	"figma", "sketch", "adobe", "photoshop", "illustrator", "premiere",
	"final cut", "logic", "garageband", "procreate", "affinity",
	"davinci resolve", "resolve",
}

// adminApps indicate administrative/housekeeping activity.
var adminApps = []string{
	"calendar", "finder", "system preferences", "system settings",
	"activity monitor", "terminal", "iterm", "notes",
}

// browserApps are browsers (used for exploration detection).
var browserApps = []string{
	"chrome", "google chrome", "safari", "firefox", "arc", "brave", "edge",
}

// ClassifySession determines the session type given recent activity data.
// recentApps is a list of app names in reverse chronological order.
// sameAppMinutes is how long the current app has been in focus.
// switchCount is the number of app switches in the last 10 minutes.
// idleSeconds is seconds since any activity was detected.
func ClassifySession(recentApps []string, sameAppMinutes float64, switchCount int, idleSeconds float64) string {
	if idleSeconds > 300 {
		return SessionIdle
	}

	// Check most-recent apps (last 5 min ≈ last several entries).
	window := recentApps
	if len(window) > 10 {
		window = window[:10]
	}

	// Communication wins if a comms app appeared recently.
	for _, app := range window {
		lower := strings.ToLower(app)
		if matchesAny(lower, communicationApps) {
			return SessionCommunication
		}
	}

	// Creative.
	for _, app := range window {
		lower := strings.ToLower(app)
		if matchesAny(lower, creativeApps) {
			return SessionCreative
		}
	}

	// Admin.
	if len(window) > 0 {
		lower := strings.ToLower(window[0])
		if matchesAny(lower, adminApps) {
			return SessionAdmin
		}
	}

	// Exploration: browser with high switch rate.
	if len(window) > 0 {
		lower := strings.ToLower(window[0])
		if matchesAny(lower, browserApps) && switchCount > 5 {
			return SessionExploration
		}
	}

	// Deep focus: same app for >10 min with <3 switches.
	if sameAppMinutes > 10 && switchCount < 3 {
		return SessionDeepFocus
	}

	// Default to exploration for browser, otherwise deep focus.
	if len(window) > 0 {
		lower := strings.ToLower(window[0])
		if matchesAny(lower, browserApps) {
			return SessionExploration
		}
	}

	return SessionDeepFocus
}

// FocusScore computes a focus score from 0.0 to 1.0.
// Formula: 1.0 - (app_switches / 10.0), clamped to [0, 1].
func FocusScore(appSwitches int) float64 {
	score := 1.0 - float64(appSwitches)/10.0
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func matchesAny(lower string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
