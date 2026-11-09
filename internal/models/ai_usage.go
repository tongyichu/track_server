package models

import "time"

// AI records contain bounded business summaries and outputs, never credentials, raw GPS or image data.
type AICall struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	StartedAt     time.Time `json:"started_at"`
	DurationMS    int64     `json:"duration_ms"`
	Status        string    `json:"status"`
	HTTPStatus    int       `json:"http_status,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	InputTokens   *int64    `json:"input_tokens"`
	OutputTokens  *int64    `json:"output_tokens"`
	CachedTokens  *int64    `json:"cached_tokens"`
	EstimatedCNY  *float64  `json:"estimated_cny"`
	PriceVersion  int64     `json:"price_version,omitempty"`
	PriceSnapshot *AIPrice  `json:"price_snapshot,omitempty"`
}

type AITask struct {
	ID              string     `json:"id" bson:"_id"`
	Revision        int64      `json:"revision"`
	Feature         string     `json:"feature"`
	Provider        string     `json:"provider"`
	Model           string     `json:"model"`
	ObjectID        string     `json:"object_id"`
	ObjectName      string     `json:"object_name"`
	Mode            string     `json:"mode"`
	Reason          string     `json:"reason"`
	Actor           string     `json:"actor"`
	RetryOf         string     `json:"retry_of,omitempty"`
	RetryTaskID     string     `json:"retry_task_id,omitempty"`
	InputVersion    string     `json:"input_version"`
	RuleVersion     string     `json:"rule_version"`
	InputSummary    string     `json:"input_summary,omitempty"`
	Status          string     `json:"status"`
	Stage           string     `json:"stage"`
	Adoption        string     `json:"adoption"`
	CancelRequested bool       `json:"cancel_requested"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	LeaseUntil      time.Time  `json:"lease_until"`
	DurationMS      int64      `json:"duration_ms"`
	Error           string     `json:"error,omitempty"`
	Calls           []AICall   `json:"calls,omitempty"`
	Result          string     `json:"result,omitempty"`
	Requests        int64      `json:"requests"`
	UsageKnown      int64      `json:"usage_known"`
	InputTokens     int64      `json:"input_tokens"`
	OutputTokens    int64      `json:"output_tokens"`
	CostKnown       int64      `json:"cost_known"`
	EstimatedCNY    float64    `json:"estimated_cny"`
}

type AIPolicy struct {
	Feature        string    `json:"feature"`
	Revision       int64     `json:"revision"`
	PauseAutomatic bool      `json:"pause_automatic"`
	DailyRequests  int64     `json:"daily_requests"`
	MaxConcurrent  int       `json:"max_concurrent"`
	TokenAlert     int64     `json:"token_alert"`
	CostAlertCNY   float64   `json:"cost_alert_cny"`
	UpdatedBy      string    `json:"updated_by"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AIPrice struct {
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	InputPerMillion  *float64  `json:"input_per_million"`
	CachedPerMillion *float64  `json:"cached_per_million"`
	OutputPerMillion *float64  `json:"output_per_million"`
	SearchPerCall    *float64  `json:"search_per_call"`
	Version          int64     `json:"version"`
	UpdatedBy        string    `json:"updated_by"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// One CAS document per feature coordinates budgets and expiring task leases across processes.
type AIControl struct {
	Policy   AIPolicy             `json:"policy"`
	Day      string               `json:"day"`
	Requests int64                `json:"requests"`
	Leases   map[string]time.Time `json:"leases"`
}

type AIFilter struct {
	From, To                                                time.Time
	Feature, Provider, Model, Status, Mode, Query, Adoption string
	ObjectID, InputVersion                                  string
	Active                                                  bool
	Cursor                                                  string
	Limit                                                   int
}

type AIPage struct {
	Items      []*AITask `json:"items"`
	Total      int64     `json:"total"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type AITotals struct {
	Tasks        int64   `json:"tasks"`
	Requests     int64   `json:"requests"`
	Running      int64   `json:"running"`
	Queued       int64   `json:"queued"`
	Success      int64   `json:"success"`
	Failed       int64   `json:"failed"`
	Cancelled    int64   `json:"cancelled"`
	Adopted      int64   `json:"adopted"`
	UsageKnown   int64   `json:"usage_known"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostKnown    int64   `json:"cost_known"`
	EstimatedCNY float64 `json:"estimated_cny"`
	DurationMS   int64   `json:"duration_ms"`
	Finished     int64   `json:"finished"`
}
type AIGroup struct {
	Key string `json:"key"`
	AITotals
}
type AIStatistics struct {
	AITotals
	Features        []AIGroup  `json:"features"`
	Models          []AIGroup  `json:"models"`
	Days            []AIGroup  `json:"days"`
	FirstRecordedAt *time.Time `json:"first_recorded_at"`
}
