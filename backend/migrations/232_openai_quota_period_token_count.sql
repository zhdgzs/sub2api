ALTER TABLE openai_quota_periods
    ADD COLUMN IF NOT EXISTS token_count BIGINT CHECK (token_count >= 0);
