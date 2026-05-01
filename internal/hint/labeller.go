package hint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sksareen/sauron/internal/store"
)

const (
	labelModel  = "openai/gpt-4o-mini"
	labelPrompt = `You are watching a person's Mac. Based on the recent activity evidence below, write a SHORT human-readable label (3-8 words) describing what they are working on RIGHT NOW.

Rules:
- Be specific: "building Sauron HINT inference engine" not "coding"
- Use present tense gerund: "building", "reviewing", "researching"
- If it's clearly communication/social: "messaging on [app]"
- If it's clearly browsing/reading: "reading about [topic]"
- If evidence is too thin or ambiguous, respond with confidence 0.0 and label "unclear"
- Respond ONLY with JSON: {"label": "...", "confidence": 0.0-1.0}

Evidence (most recent first):`
)

// Labeller runs periodic LLM calls to generate human-readable labels for hints.
type Labeller struct {
	db     *store.DB
	apiKey string
	client *http.Client
}

func NewLabeller(db *store.DB) *Labeller {
	return &Labeller{
		db:     db,
		apiKey: os.Getenv("OPENROUTER_API_KEY"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// RunLabels finds hints that need labels and fires LLM calls for them.
func (l *Labeller) RunLabels(now int64) error {
	if l.apiKey == "" {
		return nil // silent: no key configured
	}

	hints, err := store.GetHintsNeedingLabel(l.db, LabelMinEvidence, LabelCooldownSec, now)
	if err != nil {
		return err
	}

	for _, h := range hints {
		evidence, err := store.GetHintEvidence(l.db, h.ID, 12)
		if err != nil || len(evidence) < LabelMinEvidence {
			continue
		}
		label, confidence, err := l.label(h, evidence)
		if err != nil || label == "" || label == "unclear" {
			continue
		}
		_ = store.UpdateHintLabel(l.db, h.ID, label, confidence, now)
	}
	return nil
}

// label calls OpenRouter and returns a label + confidence for a hint.
func (l *Labeller) label(h store.HintRecord, evidence []store.HintEvidence) (string, float64, error) {
	var sb strings.Builder
	for i, e := range evidence {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, e.AppName, e.Summary))
	}

	messages := []map[string]string{
		{"role": "system", "content": labelPrompt + "\n" + sb.String()},
		{"role": "user", "content": "What is this person working on? Respond with JSON only."},
	}

	body, _ := json.Marshal(map[string]interface{}{
		"model":       labelModel,
		"messages":    messages,
		"max_tokens":  60,
		"temperature": 0.3,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://sauron.local")

	resp, err := l.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Choices) == 0 {
		return "", 0, fmt.Errorf("bad response: %s", string(raw))
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	// Strip markdown code fences if present.
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed struct {
		Label      string  `json:"label"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", 0, fmt.Errorf("parse label response: %w", err)
	}

	return strings.TrimSpace(parsed.Label), parsed.Confidence, nil
}
