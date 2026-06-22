-- 151_codex_inspection.sql
-- OpenAI Codex 账号巡检运行、结果与日志表。

CREATE TABLE IF NOT EXISTS codex_inspection_runs (
    id BIGSERIAL PRIMARY KEY,
    trigger_type TEXT NOT NULL DEFAULT 'manual',
    trigger_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'running',
    total_accounts INTEGER NOT NULL DEFAULT 0,
    completed_accounts INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    keep_count INTEGER NOT NULL DEFAULT 0,
    enable_count INTEGER NOT NULL DEFAULT 0,
    disable_count INTEGER NOT NULL DEFAULT 0,
    reauth_count INTEGER NOT NULL DEFAULT 0,
    delete_count INTEGER NOT NULL DEFAULT 0,
    settings_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_runs_status
    ON codex_inspection_runs(status, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_runs_trigger
    ON codex_inspection_runs(trigger_type, trigger_key);

CREATE TABLE IF NOT EXISTS codex_inspection_results (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES codex_inspection_runs(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL,
    account_name TEXT NOT NULL DEFAULT '',
    account_status_snapshot TEXT NOT NULL DEFAULT '',
    schedulable_snapshot BOOLEAN NOT NULL DEFAULT FALSE,
    proxy_id_snapshot BIGINT,
    chatgpt_account_id TEXT NOT NULL DEFAULT '',
    probe_status TEXT NOT NULL DEFAULT 'skipped',
    upstream_status_code INTEGER,
    latency_ms INTEGER,
    five_hour_used_percent DOUBLE PRECISION,
    long_window_type TEXT NOT NULL DEFAULT 'none',
    long_window_used_percent DOUBLE PRECISION,
    recommended_action TEXT NOT NULL DEFAULT 'keep',
    action_reason TEXT NOT NULL DEFAULT '',
    action_status TEXT NOT NULL DEFAULT 'none',
    action_error TEXT NOT NULL DEFAULT '',
    body_excerpt TEXT NOT NULL DEFAULT '',
    raw_rate_limit JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_results_run_id
    ON codex_inspection_results(run_id, id);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_results_account_id
    ON codex_inspection_results(account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_results_action
    ON codex_inspection_results(recommended_action, probe_status);

CREATE TABLE IF NOT EXISTS codex_inspection_logs (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES codex_inspection_runs(id) ON DELETE CASCADE,
    account_id BIGINT,
    level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL DEFAULT '',
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_logs_run_id
    ON codex_inspection_logs(run_id, id);

CREATE INDEX IF NOT EXISTS idx_codex_inspection_logs_level
    ON codex_inspection_logs(level, created_at DESC);
