package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
	LastSampledAt       time.Time  `json:"last_sampled_at,omitempty"`
}

// Reset-after headers drift as requests complete. A new quota window must move
// the reset time beyond that drift; percentage changes alone are not a reset.
const openAIQuotaResetTimeTolerance = 5 * time.Minute

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
	if snapshot.SampledAt.IsZero() {
		snapshot.SampledAt = snapshot.ObservedAt
	}
	if snapshot.SampledAt.Before(state.LastSampledAt) {
		return nil, nil
	}

	if state.StartedAt.IsZero() {
		state.StartedAt = snapshot.ObservedAt.Add(-7 * 24 * time.Hour)
		if snapshot.ResetAt != nil && snapshot.ResetAt.After(snapshot.ObservedAt) {
			state.StartedAt = snapshot.ResetAt.Add(-7 * 24 * time.Hour)
		}
	}

	newerSnapshot := state.LastPercentSnapshot.IsZero() || snapshot.ObservedAt.After(state.LastPercentSnapshot)
	if !newerSnapshot {
		if snapshot.ObservedAt.Before(state.LastPercentSnapshot) {
			snapshot.Estimate = nil
		}
		snapshot.UsedPercent = state.LastUsedPercent
		snapshot.ResetAt = state.ResetAt
	}
	if newerSnapshot && state.ResetAt != nil {
		switch {
		case snapshot.ResetAt == nil:
			// Partial snapshots must not erase the known cycle boundary.
			snapshot.ResetAt = state.ResetAt
		case snapshot.ResetAt.Before(state.ResetAt.Add(-openAIQuotaResetTimeTolerance)):
			// A shorter/model-specific window can appear in the generic bucket
			// even without a window duration. Do not let it replace the weekly
			// baseline and make a later weekly observation look like a reset.
			snapshot.UsedPercent = state.LastUsedPercent
			snapshot.ResetAt = state.ResetAt
			snapshot.Estimate = nil
		}
	}

	resetDetected := false
	previousState := state
	state.LastSampledAt = snapshot.SampledAt
	resetReason := "early_reset"
	resetStartedAt := snapshot.ObservedAt
	if newerSnapshot && !state.LastPercentSnapshot.IsZero() &&
		state.ResetAt != nil && snapshot.ResetAt != nil && snapshot.ResetAt.After(snapshot.ObservedAt) &&
		snapshot.ResetAt.After(state.ResetAt.Add(openAIQuotaResetTimeTolerance)) {
		if !state.ResetAt.After(snapshot.ObservedAt) {
			resetDetected = true
			resetReason = "window_expired"
			resetStartedAt = *state.ResetAt
		} else if state.LastUsedPercent-snapshot.UsedPercent > 2 {
			// An early reset needs both a lower ratio and a new reset window.
			resetDetected = true
		}
	}
	if resetDetected {
		// Finalize the whole half-open interval, including requests since the
		// previous throttled sample. An empty period may have no saved row yet.
		previousSampledAt := previousState.LastSampledAt
		if previousSampledAt.IsZero() {
			previousSampledAt = previousState.LastPercentSnapshot
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO openai_quota_periods (
				account_id, started_at, ended_at, reset_at, request_count, token_count,
				used_usd, used_percent, snapshot_at, created_at, updated_at
			)
			SELECT $1, $2, $3, $4, COUNT(*),
				COALESCE(SUM(input_tokens::bigint + output_tokens::bigint + cache_creation_tokens::bigint + cache_read_tokens::bigint), 0),
				COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0),
				$5, $6, NOW(), NOW()
			FROM usage_logs
			WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
			HAVING COUNT(*) > 0
			ON CONFLICT (account_id, started_at) DO UPDATE SET
				ended_at = EXCLUDED.ended_at,
				request_count = CASE WHEN EXCLUDED.request_count >= openai_quota_periods.request_count
					THEN EXCLUDED.request_count ELSE openai_quota_periods.request_count END,
				token_count = CASE WHEN EXCLUDED.request_count >= openai_quota_periods.request_count
					THEN EXCLUDED.token_count ELSE openai_quota_periods.token_count END,
				used_usd = CASE WHEN EXCLUDED.request_count >= openai_quota_periods.request_count
					THEN EXCLUDED.used_usd ELSE openai_quota_periods.used_usd END,
				updated_at = NOW()
		`, snapshot.AccountID, previousState.StartedAt, resetStartedAt,
			previousState.ResetAt, previousState.LastUsedPercent, previousSampledAt); err != nil {
			return nil, err
		}
		// Also close an existing row when all its source logs have been cleaned.
		if _, err := tx.ExecContext(ctx, `
			UPDATE openai_quota_periods SET ended_at = $2, updated_at = NOW()
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
		SnapshotAt:   snapshot.SampledAt,
		Estimate:     snapshot.Estimate,
	}
	if requestCount > 0 {
		var estimateTotal, estimateStart, estimateCost, estimatePercent, estimatedAt any
		if estimate := snapshot.Estimate; estimate != nil {
			estimateTotal, estimateStart, estimateCost = estimate.TotalCost, estimate.WindowStartedAt, estimate.WindowCost
			estimatePercent, estimatedAt = estimate.UsedPercent, estimate.SampledAt
		}
		var savedEstimate openAIQuotaEstimateColumns
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO openai_quota_periods (
				account_id, started_at, reset_at, request_count, token_count, used_usd,
				used_percent, snapshot_at, estimated_total_cost, estimate_window_started_at,
				estimate_window_cost, estimate_used_percent, estimated_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
			ON CONFLICT (account_id, started_at) DO UPDATE SET
				reset_at = EXCLUDED.reset_at,
				request_count = EXCLUDED.request_count,
				token_count = EXCLUDED.token_count,
				used_usd = EXCLUDED.used_usd,
				used_percent = EXCLUDED.used_percent,
				snapshot_at = EXCLUDED.snapshot_at,
				estimated_total_cost = COALESCE(EXCLUDED.estimated_total_cost, openai_quota_periods.estimated_total_cost),
				estimate_window_started_at = COALESCE(EXCLUDED.estimate_window_started_at, openai_quota_periods.estimate_window_started_at),
				estimate_window_cost = COALESCE(EXCLUDED.estimate_window_cost, openai_quota_periods.estimate_window_cost),
				estimate_used_percent = COALESCE(EXCLUDED.estimate_used_percent, openai_quota_periods.estimate_used_percent),
				estimated_at = COALESCE(EXCLUDED.estimated_at, openai_quota_periods.estimated_at),
				updated_at = NOW()
			RETURNING id, ended_at, created_at, updated_at,
				estimated_total_cost, estimate_window_started_at, estimate_window_cost, estimate_used_percent, estimated_at
		`, period.AccountID, period.StartedAt, period.ResetAt, period.RequestCount, tokenCount,
			period.UsedUSD, period.UsedPercent, period.SnapshotAt,
			estimateTotal, estimateStart, estimateCost, estimatePercent, estimatedAt).Scan(
			&period.ID, &period.EndedAt, &period.CreatedAt, &period.UpdatedAt,
			&savedEstimate.total, &savedEstimate.start, &savedEstimate.cost, &savedEstimate.percent, &savedEstimate.sampledAt,
		); err != nil {
			return nil, err
		}
		period.Estimate = savedEstimate.value()
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if resetDetected {
		slog.Info("openai_quota_period_history_split", "account_id", snapshot.AccountID,
			"reason", resetReason,
			"previous_used_percent", previousState.LastUsedPercent, "used_percent", snapshot.UsedPercent,
			"previous_snapshot_at", previousState.LastPercentSnapshot, "snapshot_at", snapshot.ObservedAt,
			"previous_reset_at", previousState.ResetAt, "reset_at", snapshot.ResetAt,
			"previous_started_at", previousState.StartedAt, "started_at", state.StartedAt)
	}
	return period, nil
}

func (r *openAIQuotaPeriodRepository) List(ctx context.Context, accountID int64, params pagination.PaginationParams) ([]service.OpenAIQuotaPeriod, *pagination.PaginationResult, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM openai_quota_periods WHERE account_id = $1`, accountID).Scan(&total); err != nil {
		return nil, nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, started_at, ended_at, reset_at, request_count, token_count, used_usd,
			used_percent, snapshot_at, created_at, updated_at,
			estimated_total_cost, estimate_window_started_at, estimate_window_cost, estimate_used_percent, estimated_at
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
		var estimate openAIQuotaEstimateColumns
		if err := rows.Scan(
			&period.ID, &period.AccountID, &period.StartedAt, &period.EndedAt, &period.ResetAt,
			&period.RequestCount, &period.TokenCount, &period.UsedUSD, &period.UsedPercent,
			&period.SnapshotAt, &period.CreatedAt, &period.UpdatedAt,
			&estimate.total, &estimate.start, &estimate.cost, &estimate.percent, &estimate.sampledAt,
		); err != nil {
			return nil, nil, err
		}
		period.Estimate = estimate.value()
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return periods, paginationResultFromTotal(total, params), nil
}

type openAIQuotaEstimateColumns struct {
	total, cost, percent *float64
	start, sampledAt     *time.Time
}

func (c openAIQuotaEstimateColumns) value() *service.OpenAIQuotaEstimate {
	if c.total == nil || c.start == nil || c.cost == nil || c.percent == nil || c.sampledAt == nil {
		return nil
	}
	return &service.OpenAIQuotaEstimate{
		TotalCost: *c.total, WindowStartedAt: *c.start, WindowCost: *c.cost,
		UsedPercent: *c.percent, SampledAt: *c.sampledAt,
	}
}
