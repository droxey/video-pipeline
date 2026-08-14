package domain

import "time"

type CourseConfig struct {
	SchemaVersion int            `json:"schema_version" yaml:"schema_version"`
	Slug          string         `json:"slug" yaml:"slug"`
	Profile       string         `json:"profile" yaml:"profile"`
	Language      string         `json:"language" yaml:"language"`
	PVCVoiceID    string         `json:"pvc_voice_id" yaml:"pvc_voice_id"`
	FPS           int            `json:"fps" yaml:"fps"`
	Width         int            `json:"width" yaml:"width"`
	Height        int            `json:"height" yaml:"height"`
	Music         MusicConfig    `json:"music" yaml:"music"`
	SFX           SFXConfig      `json:"sfx" yaml:"sfx"`
	Approval      ApprovalConfig `json:"approval" yaml:"approval"`
	Soup          SoupConfig     `json:"soup" yaml:"soup"`
}
type MusicConfig struct {
	Enabled         bool    `json:"enabled" yaml:"enabled"`
	Prompt          string  `json:"prompt" yaml:"prompt"`
	DurationSeconds float64 `json:"duration_seconds" yaml:"duration_seconds"`
}
type SFXConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Preset  string `json:"preset" yaml:"preset"`
}
type ApprovalConfig struct {
	RequireFinalConfirmation bool `json:"require_final_confirmation" yaml:"require_final_confirmation"`
}
type SoupConfig struct {
	ValidationEnabled   bool    `json:"validation_enabled" yaml:"validation_enabled"`
	MinConfidence       float64 `json:"min_confidence" yaml:"min_confidence"`
	BorderlineThreshold float64 `json:"borderline_threshold" yaml:"borderline_threshold"`
}
type SoupCfg = SoupConfig

type SpeakerRole string

const (
	SpeakerRoleNarrator    SpeakerRole = "narrator"
	SpeakerRoleParticipant SpeakerRole = "participant"
)

type Speaker struct {
	SourceID    string      `json:"source_id"`
	DisplayName string      `json:"display_name"`
	Role        SpeakerRole `json:"role"`
}
type Segment struct {
	ID        string  `json:"id"`
	SpeakerID string  `json:"speaker_id"`
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
	Text      string  `json:"text"`
}
type SubtitleCue struct {
	Index int     `json:"index"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type SubtitleTrack struct {
	Language string        `json:"language"`
	Format   string        `json:"format"`
	Cues     []SubtitleCue `json:"cues"`
}

type Recording struct {
	RecordingID     string    `json:"recording_id"`
	Source          string    `json:"source"`
	SourceURL       string    `json:"source_url"`
	Title           string    `json:"title"`
	RecordedAt      time.Time `json:"recorded_at"`
	DurationSeconds float64   `json:"duration_seconds"`
	Speakers        []Speaker `json:"speakers"`
	Segments        []Segment `json:"segments"`
}
type Stage string

const (
	StageShortlist Stage = "shortlist"
	StageImport    Stage = "import"
	StageParse     Stage = "parse"
	StageGenerate  Stage = "generate"
	StageAlign     Stage = "align"
	StageRender    Stage = "render"
	StageMix       Stage = "mix"
	StageMux       Stage = "mux"
)

var StageOrder = []Stage{StageShortlist, StageImport, StageParse, StageGenerate, StageAlign, StageRender, StageMix, StageMux}

type StageStatus string

const (
	StagePending     StageStatus = "pending"
	StageRunning     StageStatus = "running"
	StageDone        StageStatus = "done"
	StageFailed      StageStatus = "failed"
	StageNeedsReview StageStatus = "needs_review"
)

type StageState struct {
	Stage     Stage       `json:"stage"`
	Status    StageStatus `json:"status"`
	StartedAt *time.Time  `json:"started_at,omitempty"`
	DoneAt    *time.Time  `json:"done_at,omitempty"`
	Error     string      `json:"error,omitempty"`
}
type ManifestStatus string

const (
	ManifestRunning ManifestStatus = "running"
	ManifestFailed  ManifestStatus = "failed"
	ManifestReady   ManifestStatus = "ready"
)

type Manifest struct {
	RunID       string                `json:"run_id"`
	RecordingID string                `json:"recording_id"`
	ContentHash string                `json:"content_hash"`
	ConfigHash  string                `json:"config_hash"`
	Status      ManifestStatus        `json:"status"`
	Stages      map[Stage]*StageState `json:"stages"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}
type ApprovalRecord struct {
	Gate       string    `json:"gate"`
	Approved   bool      `json:"approved"`
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
	Notes      string    `json:"notes,omitempty"`
	Rationale  string    `json:"rationale,omitempty"`
}
type RejectionRecord struct {
	RunID       string    `json:"run_id,omitempty"`
	Stage       Stage     `json:"stage,omitempty"`
	RecordingID string    `json:"recording_id"`
	Reasons     []string  `json:"reasons"`
	RejectedAt  time.Time `json:"rejected_at"`
}
type CostStatus string

const CostReported CostStatus = "reported"

type UsageRecord struct {
	Capability   string     `json:"capability,omitempty"`
	RequestID    string     `json:"request_id,omitempty"`
	Timestamp    time.Time  `json:"timestamp,omitempty"`
	Model        string     `json:"model,omitempty"`
	Voice        string     `json:"voice,omitempty"`
	InputChars   int        `json:"input_chars,omitempty"`
	CostStatus   CostStatus `json:"cost_status,omitempty"`
	Provider     string     `json:"provider"`
	Operation    string     `json:"operation"`
	Characters   int        `json:"characters"`
	CostReported bool       `json:"cost_reported"`
	At           time.Time  `json:"at"`
}
type ReviewQueueStatus string

const (
	ReviewPending  ReviewQueueStatus = "pending"
	ReviewApproved ReviewQueueStatus = "approved"
	ReviewRejected ReviewQueueStatus = "rejected"
)

type ReviewQueueEntry struct {
	ID            string            `json:"id"`
	Gate          string            `json:"gate"`
	Status        ReviewQueueStatus `json:"status"`
	Reason        string            `json:"reason"`
	CreatedAt     time.Time         `json:"created_at"`
	ReviewerNotes *string           `json:"reviewer_notes,omitempty"`
	PlanHash      string            `json:"plan_hash,omitempty"`
	Confidence    float64           `json:"confidence,omitempty"`
	Verdict       string            `json:"verdict,omitempty"`
	Reasons       []string          `json:"reasons,omitempty"`
}

type Slide struct {
	Number          int     `json:"number"`
	Title           string  `json:"title"`
	DurationSeconds float64 `json:"duration_seconds"`
	Body            string  `json:"body"`
}
