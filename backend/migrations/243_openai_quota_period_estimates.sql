-- Store estimates using the account page's 7d window, independently of history splits.
-- Leave legacy predicted_quota_usd values untouched: they used a different basis.
ALTER TABLE openai_quota_periods
    ADD COLUMN IF NOT EXISTS estimated_total_cost NUMERIC,
    ADD COLUMN IF NOT EXISTS estimate_window_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS estimate_window_cost NUMERIC,
    ADD COLUMN IF NOT EXISTS estimate_used_percent NUMERIC,
    ADD COLUMN IF NOT EXISTS estimated_at TIMESTAMPTZ;
