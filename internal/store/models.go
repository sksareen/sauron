package store

// ClipboardItem represents a captured clipboard entry.
type ClipboardItem struct {
	ID          int64  `json:"id"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	SourceApp   string `json:"source_app"`
	BundleID    string `json:"bundle_id"`
	WindowTitle string `json:"window_title"`
	CapturedAt  int64  `json:"captured_at"`
}

// ActivityEntry represents a period of app usage.
type ActivityEntry struct {
	ID          int64  `json:"id"`
	AppName     string `json:"app_name"`
	BundleID    string `json:"bundle_id"`
	WindowTitle string `json:"window_title"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     int64  `json:"ended_at"`
	DurationMs  int64  `json:"duration_ms"`
}

// ContextSession represents a classified work session.
type ContextSession struct {
	ID           int64   `json:"id"`
	SessionType  string  `json:"session_type"`
	FocusScore   float64 `json:"focus_score"`
	StartedAt    int64   `json:"started_at"`
	EndedAt      int64   `json:"ended_at"`
	AppSwitches  int     `json:"app_switches"`
	DominantApp  string  `json:"dominant_app"`
}

// Screenshot represents a captured screenshot file reference.
type Screenshot struct {
	ID          int64  `json:"id"`
	FilePath    string `json:"file_path"`
	SourceApp   string `json:"source_app"`
	BundleID    string `json:"bundle_id"`
	WindowTitle string `json:"window_title"`
	CapturedAt  int64  `json:"captured_at"`
}

// SearchResult is a result from full-text search.
type SearchResult struct {
	ID          int64   `json:"id"`
	Type        string  `json:"type"` // "clipboard"
	Content     string  `json:"content"`
	SourceApp   string  `json:"source_app"`
	WindowTitle string  `json:"window_title"`
	CapturedAt  int64   `json:"captured_at"`
	Rank        float64 `json:"rank"`
}

// ExperienceRecord represents a logged agent experience in the experience graph.
type ExperienceRecord struct {
	ID            int64    `json:"id,omitempty"`
	TaskIntent    string   `json:"task_intent"`
	Approach      string   `json:"approach"`
	ToolsUsed     []string `json:"tools_used,omitempty"`
	FailurePoints []string `json:"failure_points,omitempty"`
	Resolution    string   `json:"resolution,omitempty"`
	Outcome       string   `json:"outcome"`
	Tags          []string `json:"tags,omitempty"`
	Source        string   `json:"source,omitempty"`
	Embedding     []byte   `json:"embedding,omitempty"`
	CreatedAt     string   `json:"created_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

// IntentTrace represents a detected outcome and the activity trace leading to it.
type IntentTrace struct {
	ID                   int64   `json:"id"`
	OutcomeType          string  `json:"outcome_type"`
	OutcomeDetail        string  `json:"outcome_detail"`
	TraceSummary         string  `json:"trace_summary"`
	Embedding            []byte  `json:"embedding,omitempty"`
	ActivityWindowMinutes int    `json:"activity_window_minutes"`
	StartedAt            int64   `json:"started_at"`
	CompletedAt          int64   `json:"completed_at"`
	RawEvents            string  `json:"raw_events,omitempty"`
}
