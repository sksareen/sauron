package hint

import "math"

const (
	// MergeGapSec is the max gap between signals to merge into the same HINT.
	MergeGapSec = 15 * 60

	// DecayLambda gives ~30min half-life: weight(t) = w0 * e^(-λt)
	// ln(2) / 1800 ≈ 0.000385
	DecayLambda = math.Ln2 / 1800.0

	// EvidenceSpikeAmount is how much weight spikes on each new evidence row.
	EvidenceSpikeAmount = 0.15

	// PauseThreshold: hints below this weight get status=paused.
	PauseThreshold = 0.10

	// AbandonThreshold: hints below this weight for >2h get abandoned.
	AbandonThreshold = 0.02

	// LabelMinEvidence is the minimum evidence rows before requesting a label.
	LabelMinEvidence = 3

	// LabelCooldownSec is the minimum time between label calls for the same hint.
	LabelCooldownSec = 120

	// WeightCap is the maximum weight a hint can have.
	WeightCap = 1.0
)

// DecayWeight computes the decayed weight given current weight and seconds elapsed.
func DecayWeight(weight float64, elapsedSec int64) float64 {
	if elapsedSec <= 0 {
		return weight
	}
	decayed := weight * math.Exp(-DecayLambda*float64(elapsedSec))
	if decayed < 0 {
		return 0
	}
	return decayed
}

// SpikeWeight adds evidence spike, capped at WeightCap.
func SpikeWeight(weight float64) float64 {
	w := weight + EvidenceSpikeAmount
	if w > WeightCap {
		return WeightCap
	}
	return w
}

// StatusFromWeight returns the appropriate status string for a given weight.
func StatusFromWeight(weight float64) string {
	if weight >= PauseThreshold {
		return "active"
	}
	if weight >= AbandonThreshold {
		return "paused"
	}
	return "abandoned"
}

// NormaliseWindowPattern strips volatile parts of window titles for merge matching.
// "factory — main.ts — Visual Studio Code" → "Visual Studio Code"
// "sauron - Brave - Sav." → "Brave"
func NormaliseWindowPattern(title, appName string) string {
	if title == "" || title == appName {
		return appName
	}
	// Take the last segment after " - " or " — " separators (usually the app name / domain)
	for _, sep := range []string{" — ", " - "} {
		parts := splitLast(title, sep)
		if len(parts) > 1 {
			last := parts[len(parts)-1]
			if last != "" && len(last) < 60 {
				return last
			}
		}
	}
	if len(title) > 60 {
		return title[:60]
	}
	return title
}

func splitLast(s, sep string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
		}
	}
	parts = append(parts, s[start:])
	return parts
}
