package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type openAIQuotaPeriodRepository struct {
	db *sql.DB
}

func NewOpenAIQuotaPeriodRepository(db *sql.DB) service.OpenAIQuotaPeriodRepository {
	return &openAIQuotaPeriodRepository{db: db}
}

type openAIQuotaPeriodState struct {
	StartedAt           time.Time  `json:"started_at"`
	ResetAt             *time.Time `json:"reset_at,omitempty"`
	LastUsedPercent     float64    `json:"last_used_percent"`
	LastPercentSnapshot time.Time  `json:"last_percent_snapshot_at"`
}

func (r *openAIQuotaPeriodRepository) Sync(ctx context.Context, snapshot service.OpenAIQuotaPeriodSnapshot) (*service.OpenAIQuotaPeriod, error) {
	if r == nil || r.db == nil || snapshot.AccountID <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var extraRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 FOR UPDATE`, snapshot.AccountID).Scan(&extraRaw); err != nil {
		return nil, err
	}
	var extra map[string]any
	if err := json.Unmarshal(extraRaw, &extra); err != nil {
		return nil, fmt.Errorf("decode account extra: %w", err)
	}
	var state openAIQuotaPeriodState
	if raw, ok := extra["openai_quota_period"]; ok {
		encoded, _ := json.Marshal(raw)
		_ = json.Unmarshal(encoded, &state)
	}

	if state.StartedAt.IsZero() {
		state.StartedAt = snapshot.ObservedAt.Add(-7 * 24 * time.Hour)
		if snapshot.ResetAt != nil && snapshot.ResetAt.After(snapshot.ObservedAt) {
			state.StartedAt = snapshot.ResetAt.Add(-7 * 24 * time.Hour)
		}
	}

	newerSnapshot := state.LastPercentSnapshot.IsZero() || snapshot.ObservedAt.After(state.LastPercentSnapshot)
	if !newerSnapshot {
		snapshot.UsedPercent = state.LastUsedPercent
		snapshot.ResetAt = state.ResetAt
	}

	resetDetected := false
	resetStartedAt := snapshot.ObservedAt
	if newerSnapshot && !state.LastPercentSnapshot.IsZero() {
		resetDetected = state.LastUsedPercent-snapshot.UsedPercent > 2
		if !resetDetected && state.ResetAt != nil && snapshot.ResetAt != nil {
			naturalReset := !state.ResetAt.After(snapshot.ObservedAt) && snapshot.ResetAt.After(*state.ResetAt)
			if naturalReset {
				resetDetected = true
				resetStartedAt = *state.ResetAt
			}
		}
	}
	if resetDetected {
		if _, err := tx.ExecContext(ctx, `
			UPDATE openai_quota_periods
			SET ended_at = $2, updated_at = NOW()
			WHERE account_id = $1 AND ended_at IS NULL
		`, snapshot.AccountID, resetStartedAt); err != nil {
			return nil, err
		}
		state.StartedAt = resetStartedAt
	}
	if newerSnapshot {
		state.LastUsedPercent = snapshot.UsedPercent
		state.LastPercentSnapshot = snapshot.ObservedAt
		state.ResetAt = snapshot.ResetAt
	}

	var requestCount int64
	var tokenCount int64
	var usedUSD float64
	if err := tx.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(input_tokens::bigint + output_tokens::bigint + cache_creation_tokens::bigint + cache_read_tokens::bigint), 0),
			COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0)
		FROM usage_logs
		WHERE account_id = $1 AND created_at >= $2
	`, snapshot.AccountID, state.StartedAt).Scan(&requestCount, &tokenCount, &usedUSD); err != nil {
		return nil, err
	}

	stateRaw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = jsonb_set(COALESCE(extra, '{}'::jsonb), '{openai_quota_period}', $2::jsonb, true)
		WHERE id = $1
	`, snapshot.AccountID, stateRaw); err != nil {
		return nil, err
	}

	period := &service.OpenAIQuotaPeriod{
		AccountID:    snapshot.AccountID,
		StartedAt:    state.StartedAt,
		ResetAt:      snapshot.ResetAt,
		RequestCount: requestCount,
		TokenCount:   &tokenCount,
		UsedUSD:      usedUSD,
		UsedPercent:  snapshot.UsedPercent,
		SnapshotAt:   snapshot.ObservedAt,
	}
	if snapshot.UsedPercent > 5 && usedUSD != 0 {
		predicted := usedUSD * 100 / snapshot.UsedPercent
		period.PredictedQuotaUSD = &predicted
	}
	if requestCount > 0 {
		var predicted any
		if period.PredictedQuotaUSD != nil {
			predicted = *period.PredictedQuotaUSD
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO openai_quota_periods (
				account_id, started_at, reset_at, request_count, token_count, used_usd,
				used_percent, predicted_quota_usd, snapshot_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
			ON CONFLICT (account_id, started_at) DO UPDATE SET
				reset_at = EXCLUDED.reset_at,
				request_count = EXCLUDED.request_count,
				token_count = EXCLUDED.token_count,
				used_usd = EXCLUDED.used_usd,
				used_percent = EXCLUDED.used_percent,
				predicted_quota_usd = EXCLUDED.predicted_quota_usd,
				snapshot_at = EXCLUDED.snapshot_at,
				updated_at = NOW()
			RETURNING id, ended_at, created_at, updated_at
		`, period.AccountID, period.StartedAt, period.ResetAt, period.RequestCount, tokenCount,
			period.UsedUSD, period.UsedPercent, predicted, period.SnapshotAt).Scan(
			&period.ID, &period.EndedAt, &period.CreatedAt, &period.UpdatedAt,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return period, nil
}

func (r *openAIQuotaPeriodRepository) GetCurrentPredictions(ctx context.Context, accountIDs []int64) (map[int64]float64, error) {
	result := make(map[int64]float64)
	if r == nil || r.db == nil || len(accountIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT account_id, predicted_quota_usd
		FROM openai_quota_periods
		WHERE account_id = ANY($1) AND ended_at IS NULL
			AND predicted_quota_usd IS NOT NULL AND predicted_quota_usd <> 0
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var accountID int64
		var predicted float64
		if err := rows.Scan(&accountID, &predicted); err != nil {
			return nil, err
		}
		result[accountID] = predicted
	}
	return result, rows.Err()
}

func (r *openAIQuotaPeriodRepository) List(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.OpenAIQuotaPeriod, *pagination.PaginationResult, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM openai_quota_periods WHERE account_id = $1`, accountID).Scan(&total); err != nil {
		return nil, nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, started_at, ended_at, reset_at, request_count, token_count, used_usd,
			used_percent, predicted_quota_usd, snapshot_at, created_at, updated_at
		FROM openai_quota_periods
		WHERE account_id = $1
		ORDER BY started_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, accountID, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	periods := make([]service.OpenAIQuotaPeriod, 0)
	for rows.Next() {
		var period service.OpenAIQuotaPeriod
		if err := rows.Scan(
			&period.ID, &period.AccountID, &period.StartedAt, &period.EndedAt, &period.ResetAt,
			&period.RequestCount, &period.TokenCount, &period.UsedUSD, &period.UsedPercent, &period.PredictedQuotaUSD,
			&period.SnapshotAt, &period.CreatedAt, &period.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return periods, paginationResultFromTotal(total, params), nil
}
