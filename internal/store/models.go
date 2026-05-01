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
	ID          int64   `json:"id"`
	SessionType string  `json:"session_type"`
	FocusScore  float64 `json:"focus_score"`
	StartedAt   int64   `json:"started_at"`
	EndedAt     int64   `json:"ended_at"`
	AppSwitches int     `json:"app_switches"`
	DominantApp string  `json:"dominant_app"`
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

// VercelLog represents a single line of runtime log from a Vercel deployment.
// It's the "live:vercel" source — peer of clipboard/activity captures.
type VercelLog struct {
	ID            int64  `json:"id"`
	DeploymentURL string `json:"deployment_url"`
	Level         string `json:"level"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	StatusCode    int    `json:"status_code"`
	Message       string `json:"message"`
	CapturedAt    int64  `json:"captured_at"`
}

// Project is a v2 inferred or user-corrected work area.
type Project struct {
	ID           int64  `json:"id"`
	ProjectKey   string `json:"project_key"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	RootHint     string `json:"root_hint"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	MetadataJSON string `json:"metadata_json,omitempty"`
}

// OpenTask tracks an active loop inside a project.
type OpenTask struct {
	ID               int64   `json:"id"`
	TaskID           string  `json:"task_id"`
	ProjectID        int64   `json:"project_id"`
	Status           string  `json:"status"`
	Goal             string  `json:"goal"`
	LastUsefulState  string  `json:"last_useful_state"`
	NextAction       string  `json:"next_action"`
	Confidence       float64 `json:"confidence"`
	StartedAt        int64   `json:"started_at"`
	UpdatedAt        int64   `json:"updated_at"`
	CompletedAt      int64   `json:"completed_at,omitempty"`
	CompletionSource string  `json:"completion_source,omitempty"`
	MetadataJSON     string  `json:"metadata_json,omitempty"`
}

// HumanTrace is the v2 trace record for re-entry, interruption, completion, or debugging.
type HumanTrace struct {
	ID           int64   `json:"id"`
	TraceID      string  `json:"trace_id"`
	TraceType    string  `json:"trace_type"`
	ProjectID    int64   `json:"project_id"`
	TaskID       string  `json:"task_id"`
	TriggerType  string  `json:"trigger_type"`
	Status       string  `json:"status"`
	StartedAt    int64   `json:"started_at"`
	CompletedAt  int64   `json:"completed_at"`
	Summary      string  `json:"summary"`
	NextAction   string  `json:"next_action"`
	Confidence   float64 `json:"confidence"`
	MetadataJSON string  `json:"metadata_json,omitempty"`
}

// HumanTraceEvent is a redacted evidence pointer for a v2 human trace.
type HumanTraceEvent struct {
	ID           int64  `json:"id"`
	TraceID      string `json:"trace_id"`
	Ts           int64  `json:"ts"`
	EventType    string `json:"event_type"`
	SourceTable  string `json:"source_table"`
	SourceID     int64  `json:"source_id"`
	Summary      string `json:"summary"`
	AppName      string `json:"app_name"`
	WindowTitle  string `json:"window_title"`
	ArtifactURI  string `json:"artifact_uri"`
	Severity     string `json:"severity"`
	MetadataJSON string `json:"metadata_json,omitempty"`
}

// HintRecord is a Human Intention Vector — a live, weighted intent inferred from raw signals.
type HintRecord struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`          // LLM-inferred, e.g. "building auth flow"
	Confidence    float64 `json:"confidence"`      // LLM confidence in the label
	Weight        float64 `json:"weight"`          // live relevance, decays over time
	Status        string  `json:"status"`          // active|paused|completed|abandoned
	DominantApp   string  `json:"dominant_app"`
	WindowPattern string  `json:"window_pattern"`  // normalised window title for merge matching
	MergeGroupID  string  `json:"merge_group_id,omitempty"`
	Embedding     []byte  `json:"embedding,omitempty"`
	StartedAt     int64   `json:"started_at"`
	LastActiveAt  int64   `json:"last_active_at"`
	LabelledAt    int64   `json:"labelled_at"`
	EvidenceCount int     `json:"evidence_count"`
}

// HintEvidence is a single raw signal linked to a HintRecord.
type HintEvidence struct {
	ID          int64  `json:"id"`
	HintID      string `json:"hint_id"`
	SourceTable string `json:"source_table"` // clipboard|activity|screenshot|session
	SourceID    int64  `json:"source_id"`
	Ts          int64  `json:"ts"`
	Summary     string `json:"summary"`
	AppName     string `json:"app_name"`
	WindowTitle string `json:"window_title"`
	Severity    string `json:"severity"`
}

// IntentTrace represents a detected outcome and the activity trace leading to it.
type IntentTrace struct {
	ID                    int64  `json:"id"`
	OutcomeType           string `json:"outcome_type"`
	OutcomeDetail         string `json:"outcome_detail"`
	TraceSummary          string `json:"trace_summary"`
	Embedding             []byte `json:"embedding,omitempty"`
	ActivityWindowMinutes int    `json:"activity_window_minutes"`
	StartedAt             int64  `json:"started_at"`
	CompletedAt           int64  `json:"completed_at"`
	RawEvents             string `json:"raw_events,omitempty"`
}
