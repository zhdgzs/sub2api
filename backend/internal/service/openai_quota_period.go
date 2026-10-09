package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const openAIQuotaPeriodTriggerInterval = 30 * time.Second

// OpenAIQuotaPeriod records one observed OpenAI long-window quota cycle.
type OpenAIQuotaPeriod struct {
	ID           int64                `json:"id"`
	AccountID    int64                `json:"account_id"`
	StartedAt    time.Time            `json:"started_at"`
	EndedAt      *time.Time           `json:"ended_at,omitempty"`
	ResetAt      *time.Time           `json:"reset_at,omitempty"`
	RequestCount int64                `json:"request_count"`
	TokenCount   *int64               `json:"token_count,omitempty"`
	UsedUSD      float64              `json:"used_usd"`
	UsedPercent  float64              `json:"used_percent"`
	SnapshotAt   time.Time            `json:"snapshot_at"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Estimate     *OpenAIQuotaEstimate `json:"estimate,omitempty"`
}

// OpenAIQuotaEstimate is the same 7d estimate returned to the account page.
// Its window can differ from the history period after an early reset.
type OpenAIQuotaEstimate struct {
	TotalCost       float64   `json:"total_cost"`
	WindowStartedAt time.Time `json:"window_started_at"`
	WindowCost      float64   `json:"window_cost"`
	UsedPercent     float64   `json:"used_percent"`
	SampledAt       time.Time `json:"sampled_at"`
}

func buildOpenAIQuotaEstimate(progress *UsageProgress, now time.Time) *OpenAIQuotaEstimate {
	if progress == nil || progress.WindowStats == nil {
		return nil
	}
	cost, percent := progress.WindowStats.Cost, progress.Utilization
	if cost <= 0 || percent <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) || math.IsNaN(percent) || math.IsInf(percent, 0) {
		return nil
	}
	total := cost * 100 / percent
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return nil
	}
	return &OpenAIQuotaEstimate{
		TotalCost: total, WindowStartedAt: codexWindowStatsStart(progress, 7*24*time.Hour, now),
		WindowCost: cost, UsedPercent: percent, SampledAt: now,
	}
}

type OpenAIQuotaPeriodSnapshot struct {
	AccountID   int64
	ObservedAt  time.Time
	UsedPercent float64
	ResetAt     *time.Time
	SampledAt   time.Time
	Estimate    *OpenAIQuotaEstimate
}

type OpenAIQuotaPeriodRepository interface {
	Sync(ctx context.Context, snapshot OpenAIQuotaPeriodSnapshot) (*OpenAIQuotaPeriod, error)
	List(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]OpenAIQuotaPeriod, *pagination.PaginationResult, error)
}

type OpenAIQuotaPeriodService struct {
	repo        OpenAIQuotaPeriodRepository
	accountRepo AccountRepository
	usageRepo   UsageLogRepository
	triggered   sync.Map
}

func NewOpenAIQuotaPeriodService(repo OpenAIQuotaPeriodRepository, accountRepo AccountRepository, usageRepo UsageLogRepository) *OpenAIQuotaPeriodService {
	return &OpenAIQuotaPeriodService{repo: repo, accountRepo: accountRepo, usageRepo: usageRepo}
}

func openAIQuotaPeriodEligible(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.GetCredential("plan_type"))) {
	case "plus", "team", "pro":
		return true
	default:
		return false
	}
}

// SupportsOpenAIQuotaPeriods reports whether an account participates in the
// OpenAI long-window quota history feature.
func SupportsOpenAIQuotaPeriods(account *Account) bool {
	return openAIQuotaPeriodEligible(account)
}

func openAIQuotaPeriodSnapshot(account *Account, now time.Time) (OpenAIQuotaPeriodSnapshot, bool) {
	if !openAIQuotaPeriodEligible(account) || len(account.Extra) == 0 {
		return OpenAIQuotaPeriodSnapshot{}, false
	}
	usedRaw, ok := account.Extra["codex_7d_used_percent"]
	if !ok {
		return OpenAIQuotaPeriodSnapshot{}, false
	}
	observedAt := now
	if raw, exists := account.Extra["codex_usage_updated_at"]; exists {
		parsed, err := parseTime(fmt.Sprint(raw))
		if err != nil {
			return OpenAIQuotaPeriodSnapshot{}, false
		}
		observedAt = parsed.UTC()
	}
	usedPercent := parseExtraFloat64(usedRaw)
	if usedPercent < 0 || usedPercent > 100 || math.IsNaN(usedPercent) || math.IsInf(usedPercent, 0) {
		return OpenAIQuotaPeriodSnapshot{}, false
	}
	var resetAt *time.Time
	if raw, exists := account.Extra["codex_7d_reset_at"]; exists {
		if parsed, err := parseTime(fmt.Sprint(raw)); err == nil {
			value := parsed.UTC()
			resetAt = &value
		}
	}
	return OpenAIQuotaPeriodSnapshot{
		AccountID:   account.ID,
		ObservedAt:  observedAt,
		UsedPercent: usedPercent,
		ResetAt:     resetAt,
		SampledAt:   now,
	}, true
}

// SyncAccount persists the latest cached long-window snapshot for quota history.
// Its period boundaries and totals do not control the account usage window.
func (s *OpenAIQuotaPeriodService) SyncAccount(ctx context.Context, account *Account) (*OpenAIQuotaPeriod, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	snapshot, ok := openAIQuotaPeriodSnapshot(account, now)
	if !ok {
		return nil, nil
	}
	if s.usageRepo != nil {
		progress := buildCodexUsageProgressFromExtra(account.Extra, "7d", now)
		stats, err := s.usageRepo.GetAccountWindowStats(ctx, account.ID, codexWindowStatsStart(progress, 7*24*time.Hour, now))
		if err != nil {
			return nil, err
		}
		if progress != nil {
			progress.WindowStats = windowStatsFromAccountStats(stats)
			snapshot.Estimate = buildOpenAIQuotaEstimate(progress, now)
		}
	}
	return s.repo.Sync(ctx, snapshot)
}

// SyncUsage saves the exact estimate assembled for the account response.
func (s *OpenAIQuotaPeriodService) SyncUsage(ctx context.Context, account *Account, estimate *OpenAIQuotaEstimate, now time.Time) (*OpenAIQuotaPeriod, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	snapshot, ok := openAIQuotaPeriodSnapshot(account, now)
	if !ok {
		return nil, nil
	}
	snapshot.Estimate = estimate
	return s.repo.Sync(ctx, snapshot)
}

// TriggerAfterUsage schedules a best-effort local refresh after a usage row has
// been persisted. It never queries OpenAI and is throttled per account.
func (s *OpenAIQuotaPeriodService) TriggerAfterUsage(accountID int64) {
	if s == nil || s.repo == nil || s.accountRepo == nil || accountID <= 0 {
		return
	}
	now := time.Now()
	for {
		previous, loaded := s.triggered.LoadOrStore(accountID, now)
		if !loaded {
			break
		}
		timestamp, valid := previous.(time.Time)
		if valid && now.Sub(timestamp) < openAIQuotaPeriodTriggerInterval {
			return
		}
		if s.triggered.CompareAndSwap(accountID, previous, now) {
			break
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		account, err := s.accountRepo.GetByID(ctx, accountID)
		if err != nil {
			slog.Warn("openai_quota_period_load_account_failed", "account_id", accountID, "error", err)
			return
		}
		if _, err := s.SyncAccount(ctx, account); err != nil {
			slog.Warn("openai_quota_period_sync_failed", "account_id", accountID, "error", err)
		}
	}()
}

func (s *OpenAIQuotaPeriodService) List(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]OpenAIQuotaPeriod, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return []OpenAIQuotaPeriod{}, &pagination.PaginationResult{}, nil
	}
	return s.repo.List(ctx, accountID, params)
}
