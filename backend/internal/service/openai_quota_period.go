package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const openAIQuotaPeriodTriggerInterval = 30 * time.Second

// OpenAIQuotaPeriod records one observed OpenAI long-window quota cycle.
type OpenAIQuotaPeriod struct {
	ID                int64      `json:"id"`
	AccountID         int64      `json:"account_id"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	ResetAt           *time.Time `json:"reset_at,omitempty"`
	RequestCount      int64      `json:"request_count"`
	TokenCount        *int64     `json:"token_count,omitempty"`
	UsedUSD           float64    `json:"used_usd"`
	UsedPercent       float64    `json:"used_percent"`
	PredictedQuotaUSD *float64   `json:"predicted_quota_usd,omitempty"`
	SnapshotAt        time.Time  `json:"snapshot_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type OpenAIQuotaPeriodSnapshot struct {
	AccountID   int64
	ObservedAt  time.Time
	UsedPercent float64
	ResetAt     *time.Time
}

type OpenAIQuotaPeriodRepository interface {
	Sync(ctx context.Context, snapshot OpenAIQuotaPeriodSnapshot) (*OpenAIQuotaPeriod, error)
	GetCurrentPredictions(ctx context.Context, accountIDs []int64) (map[int64]float64, error)
	List(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]OpenAIQuotaPeriod, *pagination.PaginationResult, error)
}

type OpenAIQuotaPeriodService struct {
	repo        OpenAIQuotaPeriodRepository
	accountRepo AccountRepository
	triggered   sync.Map
}

func NewOpenAIQuotaPeriodService(repo OpenAIQuotaPeriodRepository, accountRepo AccountRepository) *OpenAIQuotaPeriodService {
	return &OpenAIQuotaPeriodService{repo: repo, accountRepo: accountRepo}
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

func openAIQuotaPeriodSnapshot(account *Account) (OpenAIQuotaPeriodSnapshot, bool) {
	if !openAIQuotaPeriodEligible(account) || len(account.Extra) == 0 {
		return OpenAIQuotaPeriodSnapshot{}, false
	}
	usedRaw, ok := account.Extra["codex_7d_used_percent"]
	if !ok {
		return OpenAIQuotaPeriodSnapshot{}, false
	}
	observedAt := time.Now().UTC()
	if raw, exists := account.Extra["codex_usage_updated_at"]; exists {
		parsed, err := parseTime(fmt.Sprint(raw))
		if err != nil {
			return OpenAIQuotaPeriodSnapshot{}, false
		}
		observedAt = parsed.UTC()
	}
	usedPercent := parseExtraFloat64(usedRaw)
	if usedPercent < 0 || usedPercent > 100 {
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
	}, true
}

// SyncAccount persists the latest cached long-window snapshot for quota history.
// Its period boundaries and totals do not control the account usage window.
func (s *OpenAIQuotaPeriodService) SyncAccount(ctx context.Context, account *Account) (*OpenAIQuotaPeriod, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	snapshot, ok := openAIQuotaPeriodSnapshot(account)
	if !ok {
		return nil, nil
	}
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

func (s *OpenAIQuotaPeriodService) GetCurrentPredictions(ctx context.Context, accountIDs []int64) (map[int64]float64, error) {
	if s == nil || s.repo == nil || len(accountIDs) == 0 {
		return map[int64]float64{}, nil
	}
	return s.repo.GetCurrentPredictions(ctx, accountIDs)
}

func (s *OpenAIQuotaPeriodService) List(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]OpenAIQuotaPeriod, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return []OpenAIQuotaPeriod{}, &pagination.PaginationResult{}, nil
	}
	return s.repo.List(ctx, accountID, params)
}
