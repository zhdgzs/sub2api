package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	CodexInspectionTriggerManual        = "manual"
	CodexInspectionTriggerScheduled     = "scheduled"
	CodexInspectionTriggerSingleAccount = "single_account"

	CodexInspectionRunStatusRunning   = "running"
	CodexInspectionRunStatusCompleted = "completed"
	CodexInspectionRunStatusFailed    = "failed"
	CodexInspectionRunStatusCanceled  = "canceled"

	CodexInspectionProbeStatusSuccess = "success"
	CodexInspectionProbeStatusFailed  = "failed"
	CodexInspectionProbeStatusSkipped = "skipped"

	CodexInspectionActionKeep    = "keep"
	CodexInspectionActionEnable  = "enable"
	CodexInspectionActionDisable = "disable"
	CodexInspectionActionReauth  = "reauth"
	CodexInspectionActionDelete  = "delete"

	CodexInspectionActionStatusNone        = "none"
	CodexInspectionActionStatusPending     = "pending"
	CodexInspectionActionStatusSuccess     = "success"
	CodexInspectionActionStatusFailed      = "failed"
	CodexInspectionActionStatusSkipped     = "skipped"
	CodexInspectionActionStatusNeedsReview = "needs_review"

	CodexInspectionScheduleModeInterval   = "interval"
	CodexInspectionScheduleModeTimePoints = "time_points"

	CodexInspectionLongWindowNone    = "none"
	CodexInspectionLongWindowWeekly  = "weekly"
	CodexInspectionLongWindowMonthly = "monthly"
	CodexInspectionLongWindowGeneric = "generic"

	SettingKeyCodexInspectionConfig = "codex_inspection_config"

	codexInspectionUsageURL            = "https://chatgpt.com/backend-api/wham/usage"
	codexInspectionFiveHourSeconds     = 18_000
	codexInspectionWeekSeconds         = 604_800
	codexInspectionMonthSeconds        = 2_592_000
	codexInspectionMaxStoredBodyText   = 2048
	codexInspectionDefaultTimeZone     = "Asia/Shanghai"
	codexInspectionDisabledByRunIDKey  = "codex_inspection_disabled_by_run_id"
	codexInspectionLastActionKey       = "codex_last_inspection_action"
	codexInspectionLastReasonKey       = "codex_last_inspection_reason"
	codexInspectionLastRunIDKey        = "codex_last_inspection_run_id"
	codexInspectionReauthErrorMessage  = "openai codex token expired; reauth required"
	codexInspectionDeleteConfirmation  = "DELETE"
	codexInspectionDefaultResultLimit  = 50
	codexInspectionMaxResultPageSize   = 200
	codexInspectionDefaultHistoryLimit = 50
	codexInspectionDefaultWorkers      = 4
	codexInspectionMaxWorkers          = 32
	codexInspectionDefaultTimeoutMS    = 15_000
	codexInspectionMinTimeoutMS        = 1_000
	codexInspectionMaxTimeoutMS        = 120_000
)

func infraCodexInspectionError(code int, reason, message string) *infraerrors.ApplicationError {
	return infraerrors.New(code, reason, message)
}

var (
	ErrCodexInspectionRunConflict       = infraCodexInspectionError(409, "CODEX_INSPECTION_RUNNING", "a codex inspection run is already running")
	ErrCodexInspectionRunNotFound       = infraerrors.NotFound("CODEX_INSPECTION_RUN_NOT_FOUND", "codex inspection run not found")
	ErrCodexInspectionPrecondition      = infraCodexInspectionError(412, "CODEX_INSPECTION_PRECONDITION_FAILED", "codex inspection precondition failed")
	ErrCodexInspectionInvalidRequest    = infraCodexInspectionError(422, "CODEX_INSPECTION_INVALID_REQUEST", "invalid codex inspection request")
	ErrCodexInspectionUpstreamProbeFail = infraCodexInspectionError(502, "CODEX_INSPECTION_UPSTREAM_FAILED", "codex usage probe failed")
)

type CodexInspectionRepository interface {
	CreateRun(ctx context.Context, run *CodexInspectionRun) error
	UpdateRun(ctx context.Context, run *CodexInspectionRun) error
	GetRun(ctx context.Context, id int64) (*CodexInspectionRun, error)
	GetLatestRun(ctx context.Context) (*CodexInspectionRun, error)
	GetRunningRun(ctx context.Context) (*CodexInspectionRun, error)
	ListRuns(ctx context.Context, params CodexInspectionListRunsParams) ([]CodexInspectionRun, int64, error)
	InsertResult(ctx context.Context, result *CodexInspectionResult) error
	UpdateResultAction(ctx context.Context, resultID int64, status, actionError string) error
	ListResults(ctx context.Context, params CodexInspectionListResultsParams) ([]CodexInspectionResult, int64, error)
	ListLatestAccountResults(ctx context.Context, params CodexInspectionLatestResultsParams) ([]CodexInspectionResult, int64, error)
	GetResultsByIDs(ctx context.Context, ids []int64) ([]CodexInspectionResult, error)
	InsertLog(ctx context.Context, log *CodexInspectionLog) error
	ListLogs(ctx context.Context, params CodexInspectionListLogsParams) ([]CodexInspectionLog, int64, error)
	ListTargetAccounts(ctx context.Context, query CodexInspectionTargetQuery) ([]*Account, error)
	CountOpenAIOAuthAccounts(ctx context.Context) (int, error)
	CountAccountsDisabledByInspection(ctx context.Context) (int, error)
	ClearAccountExtraKeys(ctx context.Context, accountID int64, keys []string) error
}

type CodexInspectionSettings struct {
	Enabled  bool                          `json:"enabled"`
	Schedule CodexInspectionSchedule       `json:"schedule"`
	Target   CodexInspectionTargetConfig   `json:"target"`
	Probe    CodexInspectionProbeConfig    `json:"probe"`
	Decision CodexInspectionDecisionConfig `json:"decision"`
	Actions  CodexInspectionActionConfig   `json:"actions"`
}

type CodexInspectionSchedule struct {
	Mode            string   `json:"mode"`
	IntervalMinutes int      `json:"interval_minutes"`
	TimePoints      []string `json:"time_points"`
	TimeZone        string   `json:"timezone"`
}

type CodexInspectionTargetConfig struct {
	OnlyOpenAIOAuth      bool    `json:"only_openai_oauth"`
	AccountIDs           []int64 `json:"account_ids"`
	GroupIDs             []int64 `json:"group_ids"`
	IncludeUnschedulable bool    `json:"include_unschedulable"`
	IncludeError         bool    `json:"include_error"`
	OnlyStaleMinutes     int     `json:"only_stale_minutes"`
	SampleSize           int     `json:"sample_size"`
}

type CodexInspectionProbeConfig struct {
	Workers            int    `json:"workers"`
	TimeoutMS          int    `json:"timeout_ms"`
	Retries            int    `json:"retries"`
	MinIntervalMinutes int    `json:"min_interval_minutes"`
	UserAgent          string `json:"user_agent"`
}

type CodexInspectionDecisionConfig struct {
	UsedPercentThreshold float64 `json:"used_percent_threshold"`
	ShortWindowPolicy    string  `json:"short_window_policy"`
	LongWindowPolicy     string  `json:"long_window_policy"`
}

type CodexInspectionActionConfig struct {
	AutoApply       bool `json:"auto_apply"`
	AllowEnable     bool `json:"allow_enable"`
	AllowDisable    bool `json:"allow_disable"`
	AllowMarkReauth bool `json:"allow_mark_reauth"`
	AllowDelete     bool `json:"allow_delete"`
}

type CodexInspectionRun struct {
	ID                int64                   `json:"id"`
	TriggerType       string                  `json:"trigger_type"`
	TriggerKey        string                  `json:"trigger_key"`
	Status            string                  `json:"status"`
	TotalAccounts     int                     `json:"total_accounts"`
	CompletedAccounts int                     `json:"completed_accounts"`
	SuccessCount      int                     `json:"success_count"`
	ErrorCount        int                     `json:"error_count"`
	KeepCount         int                     `json:"keep_count"`
	EnableCount       int                     `json:"enable_count"`
	DisableCount      int                     `json:"disable_count"`
	ReauthCount       int                     `json:"reauth_count"`
	DeleteCount       int                     `json:"delete_count"`
	SettingsSnapshot  CodexInspectionSettings `json:"settings_snapshot"`
	StartedAt         time.Time               `json:"started_at"`
	FinishedAt        *time.Time              `json:"finished_at"`
	ErrorMessage      string                  `json:"error_message"`
}

type CodexInspectionResult struct {
	ID                    int64          `json:"id"`
	RunID                 int64          `json:"run_id"`
	AccountID             int64          `json:"account_id"`
	AccountName           string         `json:"account_name"`
	AccountStatusSnapshot string         `json:"account_status_snapshot"`
	SchedulableSnapshot   bool           `json:"schedulable_snapshot"`
	ProxyIDSnapshot       *int64         `json:"proxy_id_snapshot"`
	ChatGPTAccountID      string         `json:"chatgpt_account_id"`
	ProbeStatus           string         `json:"probe_status"`
	UpstreamStatusCode    *int           `json:"upstream_status_code"`
	LatencyMS             *int           `json:"latency_ms"`
	FiveHourUsedPercent   *float64       `json:"five_hour_used_percent"`
	LongWindowType        string         `json:"long_window_type"`
	LongWindowUsedPercent *float64       `json:"long_window_used_percent"`
	RecommendedAction     string         `json:"recommended_action"`
	ActionReason          string         `json:"action_reason"`
	ActionStatus          string         `json:"action_status"`
	ActionError           string         `json:"action_error"`
	BodyExcerpt           string         `json:"body_excerpt"`
	RawRateLimit          map[string]any `json:"raw_rate_limit"`
	CreatedAt             time.Time      `json:"created_at"`
}

type CodexInspectionLog struct {
	ID        int64          `json:"id"`
	RunID     int64          `json:"run_id"`
	AccountID *int64         `json:"account_id"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Detail    map[string]any `json:"detail"`
	CreatedAt time.Time      `json:"created_at"`
}

type CodexInspectionOverview struct {
	Settings             CodexInspectionSettings `json:"settings"`
	LatestRun            *CodexInspectionRun     `json:"latest_run"`
	RunningRun           *CodexInspectionRun     `json:"running_run"`
	TotalOpenAIOAuth     int                     `json:"total_openai_oauth"`
	HealthyAccounts      int                     `json:"healthy_accounts"`
	FiveHourFullAccounts int                     `json:"five_hour_full_accounts"`
	LongWindowFull       int                     `json:"long_window_full_accounts"`
	ReauthAccounts       int                     `json:"reauth_accounts"`
	DeleteSuggested      int                     `json:"delete_suggested_accounts"`
	DisabledByInspection int                     `json:"disabled_by_inspection_accounts"`
	ProbeFailedAccounts  int                     `json:"probe_failed_accounts"`
}

type CodexInspectionListRunsParams struct {
	Status      string
	TriggerType string
	Limit       int
	Offset      int
}

type CodexInspectionListResultsParams struct {
	RunID            int64
	Page             int
	PageSize         int
	Action           string
	ProbeStatus      string
	AccountStatus    string
	QuotaWindow      string
	GroupIDs         []int64
	OnlyStaleMinutes int
	Search           string
}

type CodexInspectionListLogsParams struct {
	RunID     int64
	AccountID int64
	Level     string
	Limit     int
	Offset    int
}

type CodexInspectionLatestResultsParams struct {
	Page             int
	PageSize         int
	Action           string
	ProbeStatus      string
	AccountStatus    string
	QuotaWindow      string
	GroupIDs         []int64
	OnlyStaleMinutes int
	Search           string
}

type CodexInspectionRunRequest struct {
	AccountIDs       []int64                   `json:"account_ids"`
	Filters          CodexInspectionRunFilters `json:"filters"`
	ApplyActions     bool                      `json:"apply_actions"`
	SettingsOverride *CodexInspectionSettings  `json:"settings_override"`
	TriggerType      string                    `json:"-"`
	TriggerKey       string                    `json:"-"`
}

type CodexInspectionRunFilters struct {
	GroupIDs             []int64 `json:"group_ids"`
	IncludeUnschedulable bool    `json:"include_unschedulable"`
	IncludeError         bool    `json:"include_error"`
	OnlyStaleMinutes     int     `json:"only_stale_minutes"`
}

type CodexInspectionActionRequest struct {
	ResultIDs        []int64 `json:"result_ids"`
	ActionOverride   string  `json:"action_override"`
	Force            bool    `json:"force"`
	ConfirmationText string  `json:"confirmation_text"`
}

type CodexInspectionActionOutcome struct {
	ResultID  int64  `json:"result_id"`
	AccountID int64  `json:"account_id"`
	Action    string `json:"action"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type CodexInspectionRunDetail struct {
	Run     *CodexInspectionRun     `json:"run"`
	Results []CodexInspectionResult `json:"results"`
	Logs    []CodexInspectionLog    `json:"logs"`
}

type CodexInspectionTargetQuery struct {
	AccountIDs           []int64
	GroupIDs             []int64
	IncludeUnschedulable bool
	IncludeError         bool
	OnlyStaleMinutes     int
	SampleSize           int
}

type CodexInspectionResultsPage struct {
	Items    []CodexInspectionResult `json:"items"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
	Pages    int                     `json:"pages"`
}

type CodexInspectionRunsPage struct {
	Items []CodexInspectionRun `json:"items"`
	Total int64                `json:"total"`
}

type CodexInspectionLogsPage struct {
	Items []CodexInspectionLog `json:"items"`
	Total int64                `json:"total"`
}

type CodexInspectionProbeOutcome struct {
	ProbeStatus           string
	StatusCode            *int
	LatencyMS             *int
	BodyExcerpt           string
	RawRateLimit          map[string]any
	Windows               []CodexInspectionRateLimitWindow
	FiveHourUsedPercent   *float64
	FiveHourWindowMinutes *int
	LongWindowType        string
	LongWindowUsedPercent *float64
	LongWindowMinutes     *int
	Error                 string
	BodyText              string
}

type CodexInspectionRateLimitWindow struct {
	WindowSeconds int            `json:"window_seconds"`
	WindowMinutes int            `json:"window_minutes"`
	UsedPercent   *float64       `json:"used_percent,omitempty"`
	Allowed       *bool          `json:"allowed,omitempty"`
	LimitReached  bool           `json:"limit_reached,omitempty"`
	Raw           map[string]any `json:"raw,omitempty"`
}

type CodexInspectionDecision struct {
	Action           string
	Reason           string
	Confidence       float64
	IsQuotaIssue     bool
	LongWindowType   string
	FiveHourFull     bool
	LongWindowFull   bool
	UnknownOrFailure bool
}
