package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIQuotaPeriodSyncKeepsEstimateIndependentOfHistorySplit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(24 * time.Hour)
	windowStart := reset.Add(-7 * 24 * time.Hour)
	state := openAIQuotaPeriodState{
		StartedAt: windowStart, ResetAt: &reset, LastUsedPercent: 50,
		LastPercentSnapshot: now.Add(-time.Minute), LastSampledAt: now.Add(-time.Minute),
	}
	extra, err := json.Marshal(map[string]any{"openai_quota_period": state})
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(extra))
	mock.ExpectExec("UPDATE openai_quota_periods").WithArgs(int64(1), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT.*COUNT").WithArgs(int64(1), now).
		WillReturnRows(sqlmock.NewRows([]string{"count", "tokens", "cost"}).AddRow(2, 200, 2))
	mock.ExpectExec("UPDATE accounts").WithArgs(int64(1), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO openai_quota_periods").WithArgs(
		int64(1), now, reset, int64(2), int64(200), 2.0, 25.0, now,
		120.0, windowStart, 30.0, 25.0, now,
	).WillReturnRows(sqlmock.NewRows([]string{
		"id", "ended_at", "created_at", "updated_at", "estimated_total_cost",
		"estimate_window_started_at", "estimate_window_cost", "estimate_used_percent", "estimated_at",
	}).AddRow(9, nil, now, now, 120, windowStart, 30, 25, now))
	mock.ExpectCommit()

	period, err := NewOpenAIQuotaPeriodRepository(db).Sync(context.Background(), service.OpenAIQuotaPeriodSnapshot{
		AccountID: 1, ObservedAt: now, SampledAt: now, UsedPercent: 25, ResetAt: &reset,
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
