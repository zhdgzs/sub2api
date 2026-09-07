CREATE TABLE IF NOT EXISTS openai_quota_periods (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    reset_at TIMESTAMPTZ,
    request_count BIGINT NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    used_usd NUMERIC(20, 8) NOT NULL DEFAULT 0 CHECK (used_usd >= 0),
    used_percent NUMERIC(8, 4) NOT NULL DEFAULT 0 CHECK (used_percent >= 0 AND used_percent <= 100),
    predicted_quota_usd NUMERIC(20, 8),
    snapshot_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT openai_quota_period_time_valid CHECK (ended_at IS NULL OR ended_at >= started_at),
    CONSTRAINT openai_quota_period_account_start_unique UNIQUE (account_id, started_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS openai_quota_period_one_active_per_account
    ON openai_quota_periods (account_id)
    WHERE ended_at IS NULL;

CREATE INDEX IF NOT EXISTS openai_quota_period_account_history
    ON openai_quota_periods (account_id, started_at DESC, id DESC);
