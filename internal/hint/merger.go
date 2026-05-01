package hint

import (
	"fmt"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

// Merger processes raw signals into HINTs and runs weight decay.
type Merger struct {
	db *store.DB
}

func NewMerger(db *store.DB) *Merger {
	return &Merger{db: db}
}

// IngestActivity merges an activity entry into an existing HINT or creates a new one.
func (m *Merger) IngestActivity(e store.ActivityEntry) error {
	if e.AppName == "" {
		return nil
	}
	windowPattern := NormaliseWindowPattern(e.WindowTitle, e.AppName)
	ts := e.StartedAt
	if e.EndedAt > 0 {
		ts = e.EndedAt
	}

	hint, err := m.findOrCreate(e.AppName, windowPattern, ts)
	if err != nil {
		return err
	}

	summary := e.AppName
	if e.WindowTitle != "" && e.WindowTitle != e.AppName {
		summary = fmt.Sprintf("%s — %s", e.AppName, e.WindowTitle)
		if len(summary) > 120 {
			summary = summary[:120]
		}
	}

	return m.appendEvidence(hint, store.HintEvidence{
		HintID:      hint.ID,
		SourceTable: "activity",
		SourceID:    e.ID,
		Ts:          ts,
		Summary:     summary,
		AppName:     e.AppName,
		WindowTitle: e.WindowTitle,
		Severity:    "info",
	})
}

// IngestClipboard merges a clipboard item into an existing HINT or creates one.
func (m *Merger) IngestClipboard(c store.ClipboardItem) error {
	if c.SourceApp == "" {
		return nil
	}
	windowPattern := NormaliseWindowPattern(c.WindowTitle, c.SourceApp)

	hint, err := m.findOrCreate(c.SourceApp, windowPattern, c.CapturedAt)
	if err != nil {
		return err
	}

	snippet := c.Content
	if len(snippet) > 80 {
		snippet = snippet[:77] + "..."
	}
	summary := fmt.Sprintf("clipboard from %s: %q", c.SourceApp, snippet)

	return m.appendEvidence(hint, store.HintEvidence{
		HintID:      hint.ID,
		SourceTable: "clipboard",
		SourceID:    c.ID,
		Ts:          c.CapturedAt,
		Summary:     summary,
		AppName:     c.SourceApp,
		WindowTitle: c.WindowTitle,
		Severity:    "info",
	})
}

// IngestSession merges a context session into an existing HINT or creates one.
func (m *Merger) IngestSession(s store.ContextSession) error {
	if s.DominantApp == "" {
		return nil
	}
	hint, err := m.findOrCreate(s.DominantApp, s.DominantApp, s.StartedAt)
	if err != nil {
		return err
	}

	summary := fmt.Sprintf("%s session, %.0f%% focus, %s", s.SessionType, s.FocusScore*100, s.DominantApp)
	return m.appendEvidence(hint, store.HintEvidence{
		HintID:      hint.ID,
		SourceTable: "session",
		SourceID:    s.ID,
		Ts:          s.StartedAt,
		Summary:     summary,
		AppName:     s.DominantApp,
		Severity:    "info",
	})
}

// RunDecay applies exponential weight decay to all active/paused hints.
func (m *Merger) RunDecay(now int64) error {
	hints, err := store.GetAllActiveHintsForDecay(m.db)
	if err != nil {
		return err
	}
	for _, h := range hints {
		elapsed := now - h.LastActiveAt
		newWeight := DecayWeight(h.Weight, elapsed)
		newStatus := StatusFromWeight(newWeight)
		if newWeight == h.Weight && newStatus == h.Status {
			continue
		}
		if err := store.UpdateHintWeight(m.db, h.ID, newWeight, newStatus); err != nil {
			return err
		}
	}
	return nil
}

// findOrCreate finds a matching active HINT or creates a new one.
func (m *Merger) findOrCreate(app, windowPattern string, ts int64) (*store.HintRecord, error) {
	mergeAfter := ts - MergeGapSec
	candidate, err := store.FindMergeCandidate(m.db, app, windowPattern, mergeAfter)
	if err != nil {
		return nil, err
	}
	if candidate != nil {
		return candidate, nil
	}

	// Create new HINT.
	now := time.Now().Unix()
	id := fmt.Sprintf("hint_%d", now*1_000_000_000+int64(ts%1_000_000_000))
	h := &store.HintRecord{
		ID:            id,
		Label:         "",
		Confidence:    0,
		Weight:        WeightCap,
		Status:        "active",
		DominantApp:   app,
		WindowPattern: windowPattern,
		StartedAt:     ts,
		LastActiveAt:  ts,
		LabelledAt:    0,
		EvidenceCount: 0,
	}
	if err := store.UpsertHint(m.db, h); err != nil {
		return nil, fmt.Errorf("create hint: %w", err)
	}
	return h, nil
}

// appendEvidence adds evidence and spikes the hint weight.
func (m *Merger) appendEvidence(h *store.HintRecord, e store.HintEvidence) error {
	if err := store.AppendHintEvidence(m.db, &e); err != nil {
		return err
	}
	newWeight := SpikeWeight(h.Weight)
	newStatus := StatusFromWeight(newWeight)
	return store.UpdateHintWeight(m.db, h.ID, newWeight, newStatus)
}
