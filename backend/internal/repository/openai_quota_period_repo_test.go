package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type quotaPeriodStateArgument struct {
	state *openAIQuotaPeriodState
}

func (a quotaPeriodStateArgument) Match(value driver.Value) bool {
	raw, ok := value.([]byte)
	return ok && json.Unmarshal(raw, a.state) == nil
}

func TestOpenAIQuotaPeriodSyncRequiresANewResetWindow(t *testing.T) {
	now := time.Date(2026, 10, 10, 1, 0, 0, 0, time.UTC)
	reset := now.Add(4 * 24 * time.Hour)
	expired := now.Add(-time.Minute)
	shift := func(at time.Time, delta time.Duration) *time.Time {
		value := at.Add(delta)
		return &value
	}
	tests := []struct {
		name       string
		previous   *time.Time
		next       *time.Time
		percent    float64
		observedAt time.Time
		split      bool
		preserve   bool
	}{
		{name: "100 to 1 in the same window", previous: &reset, next: &reset, percent: 1},
		{name: "100 to 0 in the same window", previous: &reset, next: &reset, percent: 0},
		{name: "forward header drift", previous: &reset, next: shift(reset, time.Minute), percent: 1},
		{name: "exactly five minutes is drift", previous: &reset, next: shift(reset, 5*time.Minute), percent: 1},
		{name: "backward header drift", previous: &reset, next: shift(reset, -time.Minute), percent: 1},
		{name: "shorter window cannot replace weekly baseline", previous: &reset, next: shift(reset, -3*24*time.Hour), percent: 1, preserve: true},
		{name: "missing reset preserves boundary", previous: &reset, percent: 1},
		{name: "unknown previous reset is not confirmation", next: &reset, percent: 1},
		{name: "new reset without percent drop", previous: &reset, next: shift(reset, 24*time.Hour), percent: 100},
		{name: "two point drop is not early reset", previous: &reset, next: shift(reset, 24*time.Hour), percent: 98},
		{name: "confirmed early reset", previous: &reset, next: shift(reset, 3*24*time.Hour), percent: 1, split: true},
		{name: "natural expiry with percent drop", previous: &expired, next: &reset, percent: 1, split: true},
		{name: "natural expiry without percent drop", previous: &expired, next: &reset, percent: 100, split: true},
		{name: "expired snapshot has no new active window", previous: &expired, next: shift(now, -time.Second), percent: 1},
		{name: "older observation cannot split", previous: &reset, next: shift(reset, 24*time.Hour), percent: 1, observedAt: now.Add(-2 * time.Minute), preserve: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			start := now.Add(-3 * 24 * time.Hour)
			previousSample := now.Add(-time.Minute)
			state := openAIQuotaPeriodState{
				StartedAt: start, ResetAt: tt.previous, LastUsedPercent: 100,
				LastPercentSnapshot: previousSample, LastSampledAt: previousSample,
			}
			extra, err := json.Marshal(map[string]any{"openai_quota_period": state})
			require.NoError(t, err)
			observedAt := tt.observedAt
			if observedAt.IsZero() {
				observedAt = now
			}
			expectedStart := start
			if tt.split {
				expectedStart = observedAt
				if !tt.previous.After(observedAt) {
					expectedStart = *tt.previous
				}
			}
			expectedReset, expectedPercent := tt.next, tt.percent
			if tt.next == nil || tt.preserve {
				expectedReset = tt.previous
			}
			if tt.preserve {
				expectedPercent = state.LastUsedPercent
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(1)).
				WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(extra))
			if tt.split {
				mock.ExpectExec("INSERT INTO openai_quota_periods").WithArgs(
					int64(1), start, expectedStart, tt.previous, 100.0, previousSample,
				).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE openai_quota_periods").WithArgs(int64(1), expectedStart).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectQuery("SELECT.*COUNT").WithArgs(int64(1), expectedStart).
				WillReturnRows(sqlmock.NewRows([]string{"count", "tokens", "cost"}).AddRow(2, 200, 2))
			var savedState openAIQuotaPeriodState
			mock.ExpectExec("UPDATE accounts").WithArgs(int64(1), quotaPeriodStateArgument{&savedState}).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("INSERT INTO openai_quota_periods").WithArgs(
				int64(1), expectedStart, expectedReset, int64(2), int64(200), 2.0, expectedPercent, now,
				nil, nil, nil, nil, nil,
			).WillReturnRows(sqlmock.NewRows([]string{
				"id", "ended_at", "created_at", "updated_at", "estimated_total_cost",
				"estimate_window_started_at", "estimate_window_cost", "estimate_used_percent", "estimated_at",
			}).AddRow(1, nil, now, now, nil, nil, nil, nil, nil))
			mock.ExpectCommit()

			period, err := NewOpenAIQuotaPeriodRepository(db).Sync(context.Background(), service.OpenAIQuotaPeriodSnapshot{
				AccountID: 1, ObservedAt: observedAt, SampledAt: now, UsedPercent: tt.percent, ResetAt: tt.next,
			})
			require.NoError(t, err)
			require.Equal(t, expectedStart, period.StartedAt)
			require.Equal(t, expectedReset, period.ResetAt)
			require.Equal(t, expectedPercent, period.UsedPercent)
			require.Equal(t, expectedStart, savedState.StartedAt)
			require.Equal(t, expectedReset, savedState.ResetAt)
			require.Equal(t, expectedPercent, savedState.LastUsedPercent)
			require.Equal(t, now, savedState.LastSampledAt)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestOpenAIQuotaPeriodSyncKeepsEstimateIndependentOfHistorySplit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(24 * time.Hour)
	nextReset := now.Add(7 * 24 * time.Hour)
	windowStart := reset.Add(-7 * 24 * time.Hour)
	state := openAIQuotaPeriodState{
		StartedAt: windowStart, ResetAt: &reset, LastUsedPercent: 50,
		LastPercentSnapshot: now.Add(-time.Minute), LastSampledAt: now.Add(-time.Minute),
	}
	extra, err := json.Marshal(map[string]any{"openai_quota_period": state})
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(extra))
	mock.ExpectExec("INSERT INTO openai_quota_periods").WithArgs(
		int64(1), windowStart, now, reset, 50.0, now.Add(-time.Minute),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE openai_quota_periods").WithArgs(int64(1), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT.*COUNT").WithArgs(int64(1), now).
		WillReturnRows(sqlmock.NewRows([]string{"count", "tokens", "cost"}).AddRow(2, 200, 2))
	mock.ExpectExec("UPDATE accounts").WithArgs(int64(1), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO openai_quota_periods").WithArgs(
		int64(1), now, nextReset, int64(2), int64(200), 2.0, 25.0, now,
		120.0, windowStart, 30.0, 25.0, now,
	).WillReturnRows(sqlmock.NewRows([]string{
		"id", "ended_at", "created_at", "updated_at", "estimated_total_cost",
		"estimate_window_started_at", "estimate_window_cost", "estimate_used_percent", "estimated_at",
	}).AddRow(9, nil, now, now, 120, windowStart, 30, 25, now))
	mock.ExpectCommit()

	period, err := NewOpenAIQuotaPeriodRepository(db).Sync(context.Background(), service.OpenAIQuotaPeriodSnapshot{
		AccountID: 1, ObservedAt: now, SampledAt: now, UsedPercent: 25, ResetAt: &nextReset,
		Estimate: &service.OpenAIQuotaEstimate{
			TotalCost: 120, WindowStartedAt: windowStart, WindowCost: 30, UsedPercent: 25, SampledAt: now,
		},
	})
	require.NoError(t, err)
	require.Equal(t, now, period.StartedAt)
	require.Equal(t, 2.0, period.UsedUSD)
	require.Equal(t, 120.0, period.Estimate.TotalCost) // full-window 30 / 25%, not history-period 2 / 25%
	require.Equal(t, windowStart, period.Estimate.WindowStartedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenAIQuotaPeriodSyncIgnoresOutOfOrderSample(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC()
	extra, err := json.Marshal(map[string]any{"openai_quota_period": openAIQuotaPeriodState{LastSampledAt: now}})
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(extra))
	mock.ExpectRollback()
	period, err := NewOpenAIQuotaPeriodRepository(db).Sync(context.Background(), service.OpenAIQuotaPeriodSnapshot{
		AccountID: 1, ObservedAt: now.Add(-time.Minute), SampledAt: now.Add(-time.Minute), UsedPercent: 40,
	})
	require.NoError(t, err)
	require.Nil(t, period)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenAIQuotaPeriodNaturalResetFinalizesAtExpiryEvenWhenPercentDrops(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Second)
	expiry := now.Add(-time.Minute)
	start := expiry.Add(-7 * 24 * time.Hour)
	previousSample := now.Add(-2 * time.Minute)
	nextReset := expiry.Add(7 * 24 * time.Hour)
	extra, err := json.Marshal(map[string]any{"openai_quota_period": openAIQuotaPeriodState{
		StartedAt: start, ResetAt: &expiry, LastUsedPercent: 75,
		LastPercentSnapshot: previousSample, LastSampledAt: previousSample,
	}})
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(extra))
	mock.ExpectExec("INSERT INTO openai_quota_periods").WithArgs(
		int64(1), start, expiry, expiry, 75.0, previousSample,
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE openai_quota_periods").WithArgs(int64(1), expiry).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT.*COUNT").WithArgs(int64(1), expiry).
		WillReturnRows(sqlmock.NewRows([]string{"count", "tokens", "cost"}).AddRow(0, 0, 0))
	mock.ExpectExec("UPDATE accounts").WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	period, err := NewOpenAIQuotaPeriodRepository(db).Sync(context.Background(), service.OpenAIQuotaPeriodSnapshot{
		AccountID: 1, ObservedAt: now, SampledAt: now, UsedPercent: 5, ResetAt: &nextReset,
	})
	require.NoError(t, err)
	require.Equal(t, expiry, period.StartedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpenAIQuotaPeriodListReadsSavedEstimatesWithoutRecalculation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-7 * 24 * time.Hour)
	mock.ExpectQuery("SELECT COUNT").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT id, account_id").WithArgs(int64(1), 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "started_at", "ended_at", "reset_at", "request_count", "token_count",
			"used_usd", "used_percent", "snapshot_at", "created_at", "updated_at",
			"estimated_total_cost", "estimate_window_started_at", "estimate_window_cost", "estimate_used_percent", "estimated_at",
		}).AddRow(9, 1, start, now, now, 2, 200, 2, 25, now, start, now, 120, start, 30, 25, now).
			AddRow(8, 1, start, now, now, 1, nil, 1, 0, now, start, now, nil, nil, nil, nil, nil))
	periods, page, err := NewOpenAIQuotaPeriodRepository(db).List(context.Background(), 1, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(2), page.Total)
	require.Equal(t, 120.0, periods[0].Estimate.TotalCost)
	require.Equal(t, now, periods[0].Estimate.SampledAt)
	require.Nil(t, periods[1].Estimate)
	require.Nil(t, periods[1].TokenCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
