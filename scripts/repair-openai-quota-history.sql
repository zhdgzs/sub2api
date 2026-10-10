-- Run with psql -X -v ON_ERROR_STOP=1 -v apply=false -f this-file.sql to preview.
-- Use -v apply=true only after reviewing the preview. No upstream calls.
\if :{?apply}
\else
\set apply false
\endif
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- Sync takes the account lock first; use the same order to avoid deadlocks.
SELECT id AS locked_account FROM accounts
WHERE id IN (SELECT account_id FROM openai_quota_periods) ORDER BY id FOR UPDATE;
LOCK TABLE openai_quota_periods IN SHARE ROW EXCLUSIVE MODE;

-- Keep the first pre-repair version, including removed fragment rows. These
-- backups contain no credentials and deliberately have no cascading FK.
CREATE TABLE IF NOT EXISTS openai_quota_history_repair_backup (
    period_id BIGINT PRIMARY KEY,
    original_row JSONB NOT NULL,
    backed_up_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO openai_quota_history_repair_backup (period_id, original_row)
SELECT id, to_jsonb(p) FROM openai_quota_periods p ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS openai_quota_history_state_repair_backup (
    account_id BIGINT PRIMARY KEY,
    original_state JSONB,
    backed_up_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO openai_quota_history_state_repair_backup (account_id, original_state)
SELECT id, extra->'openai_quota_period' FROM accounts
WHERE id IN (SELECT account_id FROM openai_quota_periods) ON CONFLICT DO NOTHING;

-- Collapse only contiguous fragments of the same upstream reset window.
-- The five-minute allowance covers second-resolution reset-after header drift.
-- A reset shifted by hours/days starts a separate group, preserving real cycles.
CREATE TEMP TABLE quota_repair_members ON COMMIT DROP AS
WITH previous AS (
    SELECT p.*,
        LAG(reset_at) OVER w AS previous_reset,
        LAG(ended_at) OVER w AS previous_end
    FROM openai_quota_periods p
    WINDOW w AS (PARTITION BY account_id ORDER BY started_at, id)
), boundaries AS (
    SELECT *, CASE WHEN previous_end = started_at
        AND reset_at IS NOT NULL AND previous_reset IS NOT NULL
        AND ABS(EXTRACT(EPOCH FROM reset_at - previous_reset)) <= 300
        THEN 0 ELSE 1 END AS new_group
    FROM previous
)
SELECT *, SUM(new_group) OVER (PARTITION BY account_id ORDER BY started_at, id) AS group_no
FROM boundaries;

CREATE TEMP TABLE quota_repair_plan ON COMMIT DROP AS
WITH groups AS (
    SELECT account_id, group_no, MIN(started_at) AS started_at,
        (ARRAY_AGG(id ORDER BY started_at, id))[1] AS keep_id,
        (ARRAY_AGG(id ORDER BY started_at DESC, id DESC))[1] AS latest_id,
        COUNT(*) AS fragments, SUM(request_count) AS saved_requests
    FROM quota_repair_members GROUP BY account_id, group_no
)
SELECT g.*, latest.ended_at, latest.reset_at, latest.used_percent,
    latest.snapshot_at, latest.estimated_total_cost,
    latest.estimate_window_started_at, latest.estimate_window_cost,
    latest.estimate_used_percent, latest.estimated_at,
    actual.requests, actual.tokens, actual.cost,
    estimate.cost AS reconstructed_window_cost,
    CASE WHEN latest.reset_at > latest.snapshot_at
         THEN latest.reset_at - INTERVAL '168 hours'
         ELSE latest.snapshot_at - INTERVAL '168 hours' END AS reconstructed_window_start
FROM groups g JOIN openai_quota_periods latest ON latest.id = g.latest_id
CROSS JOIN LATERAL (
    SELECT COUNT(*) AS requests,
        COALESCE(SUM(input_tokens::bigint + output_tokens::bigint +
            cache_creation_tokens::bigint + cache_read_tokens::bigint), 0) AS tokens,
        COALESCE(SUM(COALESCE(account_stats_cost, total_cost) *
            COALESCE(account_rate_multiplier, 1)), 0) AS cost
    FROM usage_logs u WHERE u.account_id = g.account_id
        AND u.created_at >= g.started_at
        AND u.created_at < COALESCE(latest.ended_at, NOW())
) actual
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(COALESCE(account_stats_cost, total_cost) *
        COALESCE(account_rate_multiplier, 1)), 0) AS cost
    FROM usage_logs u WHERE u.account_id = g.account_id
        AND u.created_at >= CASE WHEN latest.reset_at > latest.snapshot_at
            THEN latest.reset_at - INTERVAL '168 hours'
            ELSE latest.snapshot_at - INTERVAL '168 hours' END
        AND u.created_at <= latest.snapshot_at
) estimate;

-- A partial cleanup must not silently replace saved totals with smaller totals.
-- Fail the entire repair so the operator can investigate missing source logs.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM quota_repair_plan WHERE requests < saved_requests) THEN
        RAISE EXCEPTION 'Source logs are incomplete; no history changes applied';
    END IF;
END $$;

SELECT account_id, COUNT(*) AS repaired_periods, SUM(fragments) AS original_periods,
    SUM(requests) AS requests, ROUND(SUM(cost), 8) AS actual_cost
FROM quota_repair_plan GROUP BY account_id ORDER BY account_id;
SELECT keep_id, started_at, ended_at, fragments, requests,
    ROUND(cost, 8) AS actual_cost, used_percent,
    ROUND(COALESCE(estimated_total_cost,
        reconstructed_window_cost * 100 / NULLIF(used_percent, 0)), 2) AS estimated_total
FROM quota_repair_plan ORDER BY account_id, started_at;

-- Delete redundant fragments before promoting an earlier row to active status.
DELETE FROM openai_quota_periods p USING quota_repair_members m, quota_repair_plan plan
WHERE p.id = m.id AND m.account_id = plan.account_id AND m.group_no = plan.group_no
    AND p.id <> plan.keep_id;
UPDATE openai_quota_periods p SET
    ended_at = plan.ended_at, reset_at = plan.reset_at,
    request_count = plan.requests, token_count = plan.tokens, used_usd = plan.cost,
    used_percent = plan.used_percent, snapshot_at = plan.snapshot_at,
    estimated_total_cost = plan.estimated_total_cost,
    estimate_window_started_at = plan.estimate_window_started_at,
    estimate_window_cost = plan.estimate_window_cost,
    estimate_used_percent = plan.estimate_used_percent,
    estimated_at = plan.estimated_at, updated_at = NOW()
FROM quota_repair_plan plan WHERE p.id = plan.keep_id;

-- Reconstruct legacy estimates from full-window logs at the saved percentage
-- observation, never from a fragment's used_usd or legacy predicted_quota_usd.
-- Expired snapshots do not imply a positive current estimate.
UPDATE openai_quota_periods p SET
    estimated_total_cost = plan.reconstructed_window_cost * 100 / plan.used_percent,
    estimate_window_started_at = plan.reconstructed_window_start,
    estimate_window_cost = plan.reconstructed_window_cost,
    estimate_used_percent = plan.used_percent, estimated_at = plan.snapshot_at
FROM quota_repair_plan plan WHERE p.id = plan.keep_id
    AND p.estimated_total_cost IS NULL AND p.estimated_at IS NULL
    AND plan.used_percent > 0 AND plan.reconstructed_window_cost > 0
    AND (plan.reset_at IS NULL OR plan.reset_at > plan.snapshot_at);

-- Preserve the latest observation and sampling clocks; only move the active
-- boundary so the running service continues accumulating the merged cycle.
UPDATE accounts a SET extra = jsonb_set(a.extra,
    '{openai_quota_period,started_at}', to_jsonb(plan.started_at), true)
FROM quota_repair_plan plan WHERE a.id = plan.account_id AND plan.ended_at IS NULL
    AND a.extra->'openai_quota_period' IS NOT NULL;

-- Fail before commit if an interval overlaps or any current row lost its state.
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM (
            SELECT started_at, LAG(ended_at) OVER (
                PARTITION BY account_id ORDER BY started_at, id) AS previous_end
            FROM openai_quota_periods
        ) p WHERE previous_end > started_at
    ) THEN RAISE EXCEPTION 'Repaired history contains overlapping periods'; END IF;
END $$;

SELECT COUNT(*) AS final_periods,
    COUNT(*) FILTER (WHERE estimated_total_cost IS NOT NULL) AS visible_predictions,
    COUNT(*) FILTER (WHERE token_count IS NULL) AS missing_tokens
FROM openai_quota_periods;
\if :apply
COMMIT;
\else
ROLLBACK;
\endif
