-- 风控中心：本地 Prompt 规则命中详情与封禁计数排除标记

ALTER TABLE content_moderation_logs
  ADD COLUMN IF NOT EXISTS local_rule_detail JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE content_moderation_logs
  ADD COLUMN IF NOT EXISTS exclude_from_auto_ban_count BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_content_moderation_logs_local_rule_action_created_at
  ON content_moderation_logs(action, created_at DESC)
  WHERE action IN ('local_rule_hit', 'local_rule_block');
