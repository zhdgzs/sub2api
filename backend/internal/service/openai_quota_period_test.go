package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIQuotaEstimate(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(24 * time.Hour)
	for _, tc := range []struct {
		name                string
		cost, percent, want float64
	}{
		{"normal", 12, 40, 30},
		{"below former threshold", 1, 1, 100},
		{"at former threshold", 2, 5, 40},
		{"zero cost", 0, 40, 0},
		{"zero percent", 12, 0, 0},
		{"negative percent", 12, -1, 0},
		{"negative cost", -1, 40, 0},
		{"nonfinite cost", math.Inf(1), 40, 0},
		{"nonfinite percent", 12, math.NaN(), 0},
		{"overflow", math.MaxFloat64, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			progress := &UsageProgress{Utilization: tc.percent, ResetsAt: &reset, WindowStats: &WindowStats{Cost: tc.cost}}
			estimate := buildOpenAIQuotaEstimate(progress, now)
			if tc.want == 0 {
				require.Nil(t, estimate)
				return
			}
			require.Equal(t, &OpenAIQuotaEstimate{
				TotalCost: tc.want, WindowStartedAt: reset.Add(-7 * 24 * time.Hour),
				WindowCost: tc.cost, UsedPercent: tc.percent, SampledAt: now,
			}, estimate)
		})
	}
	require.Nil(t, buildOpenAIQuotaEstimate(nil, now))
	require.Nil(t, buildOpenAIQuotaEstimate(&UsageProgress{Utilization: 50}, now))
	estimate := buildOpenAIQuotaEstimate(&UsageProgress{Utilization: 50, WindowStats: &WindowStats{Cost: 12}}, now)
	require.Equal(t, now.Add(-7*24*time.Hour), estimate.WindowStartedAt)
}

type quotaEstimateUsageRepo struct {
	UsageLogRepository
	starts []time.Time
	err    error
}

func (r *quotaEstimateUsageRepo) GetAccountWindowStats(_ context.Context, _ int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts = append(r.starts, start)
	return &usagestats.AccountStats{Requests: 2, Tokens: 200, Cost: 12}, r.err
}

type quotaEstimatePeriodRepo struct {
	OpenAIQuotaPeriodRepository
	snapshot OpenAIQuotaPeriodSnapshot
	calls    int
	err      error
}

func (r *quotaEstimatePeriodRepo) Sync(_ context.Context, snapshot OpenAIQuotaPeriodSnapshot) (*OpenAIQuotaPeriod, error) {
	r.snapshot, r.calls = snapshot, r.calls+1
	return nil, r.err
}

func quotaEstimateAccount(now time.Time) *Account {
	return &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "pro"},
		Extra: map[string]any{
			"codex_5h_used_percent": 20, "codex_7d_used_percent": 40,
			"codex_7d_reset_at":      now.Add(time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at": now.Format(time.RFC3339),
		},
	}
}

func TestOpenAIQuotaHistorySkipsNonWeeklyLongWindow(t *testing.T) {
	now := time.Now().UTC()
	for _, minutes := range []int{300, 1440, 10080, 0} {
		account := quotaEstimateAccount(now)
		account.Extra["codex_7d_window_minutes"] = minutes
		repo := &quotaEstimatePeriodRepo{}
		svc := NewOpenAIQuotaPeriodService(repo, nil, nil)
		_, err := svc.SyncUsage(context.Background(), account, nil, now)
		require.NoError(t, err)
		if minutes == 10080 || minutes == 0 {
			require.Equal(t, 1, repo.calls) // retain compatibility with legacy snapshots
		} else {
			require.Zero(t, repo.calls) // daily/model limits must not split weekly history
		}
	}
}

func TestOpenAIQuotaEstimatePageAndHistoryShareSnapshot(t *testing.T) {
	for _, saveErr := range []error{nil, errors.New("history unavailable")} {
		now := time.Now().UTC()
		usageRepo := &quotaEstimateUsageRepo{}
		periodRepo := &quotaEstimatePeriodRepo{err: saveErr}
		periodService := NewOpenAIQuotaPeriodService(periodRepo, nil, usageRepo)
		svc := &AccountUsageService{usageLogRepo: usageRepo, openAIQuotaPeriodService: periodService}
		usage, err := svc.getOpenAIUsage(context.Background(), quotaEstimateAccount(now), false)
		require.NoError(t, err)
		require.NotNil(t, usage.SevenDay.EstimatedTotalCost)
		require.Equal(t, 30.0, *usage.SevenDay.EstimatedTotalCost)
		require.Equal(t, *usage.SevenDay.EstimatedTotalCost, periodRepo.snapshot.Estimate.TotalCost)
		require.Equal(t, usage.SevenDay.WindowStats.Cost, periodRepo.snapshot.Estimate.WindowCost)
		require.Equal(t, usageRepo.starts[1], periodRepo.snapshot.Estimate.WindowStartedAt)
		require.Len(t, usageRepo.starts, 2) // history reuses the page's 7d aggregate
		require.Nil(t, usage.FiveHour.EstimatedTotalCost)
	}
}

func TestOpenAIQuotaEstimateBackgroundUsesSameWindow(t *testing.T) {
	now := time.Now().UTC()
	account := quotaEstimateAccount(now)
	usageRepo := &quotaEstimateUsageRepo{}
	periodRepo := &quotaEstimatePeriodRepo{}
	svc := NewOpenAIQuotaPeriodService(periodRepo, nil, usageRepo)
	_, err := svc.SyncAccount(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 30.0, periodRepo.snapshot.Estimate.TotalCost)
	require.Equal(t, usageRepo.starts[0], periodRepo.snapshot.Estimate.WindowStartedAt)

	account.Extra["codex_7d_reset_at"] = now.Add(-time.Hour).Format(time.RFC3339)
	_, err = svc.SyncAccount(context.Background(), account)
	require.NoError(t, err)
	require.Nil(t, periodRepo.snapshot.Estimate)            // expired windows have synthetic 0% usage
	require.Equal(t, 40.0, periodRepo.snapshot.UsedPercent) // raw ratio still controls history splits

	usageRepo.err = errors.New("statistics unavailable")
	_, err = svc.SyncAccount(context.Background(), account)
	require.Error(t, err)
	require.Equal(t, 2, periodRepo.calls)
}
