# 刷新令牌同步账号套餐和额度 - Technical Design

## Architecture

本任务只改后端刷新链路，不改新增账号 RT 验证流程，不改单账号成功响应结构。

涉及边界：
- `service.OpenAIOAuthService`: 继续负责 token refresh 和 `plan_type` enrichment。
- `service.AccountUsageService` / shared helper: 复用现有 Codex probe 与 `ParseCodexRateLimitHeaders` / `buildCodexUsageExtraUpdates`。
- `handler/admin.AccountHandler`: 手动单账号刷新和批量刷新等待 quota probe，成功响应仍返回账号对象；批量使用已有 `warnings`。
- `handler/admin.OpenAIOAuthHandler`: OpenAI 专用账号刷新接口也等待 quota probe 并返回账号对象。
- `service.TokenRefreshService`: 后台自动刷新成功后同步执行短超时 best-effort quota probe；失败不改变刷新结果。
- `repository.AccountRepository`: 增加明确删除 Codex 快照 key 的能力，不能用 `UpdateExtra` 的 JSONB 合并模拟删除。

## Data Flow

手动刷新：
1. 读取旧账号，记录旧 `credentials.plan_type` 的规范化值。
2. 调用现有 OpenAI refresh/enrichment，构建并持久化 credentials。
3. 读取/使用更新后的账号和新 `plan_type`，规范化比较旧值和新值。
4. 对 OpenAI OAuth 账号执行 30 秒 Codex quota probe。
5. probe 成功且返回 headers：写入 `extra.codex_*`，合并到响应账号。
6. probe 失败/超时/无 header 且 `plan_type` 确认变化：删除旧 Codex 快照 key，响应账号也移除这些 key。
7. probe 失败/超时/无 header 且 `plan_type` 未变化：保留旧 `extra`。

后台自动刷新：
1. 现有 `TokenRefreshService` 周期扫描和 `OAuthRefreshAPI.RefreshIfNeeded` 逻辑保持不变。
2. OpenAI OAuth refresh 成功后，使用新 access token 执行 30 秒 best-effort Codex quota probe。
3. 成功则更新 `extra.codex_*`；失败且 plan 变化则清理旧快照；失败不返回错误给 token refresh。
4. 继续执行现有缓存失效、调度器缓存同步、privacy 逻辑；必要时在清理/更新 extra 后同步 fresh account。

## Contracts

`plan_type` 变化比较：
- 输入为旧 credentials 值与新 enrichment 返回值。
- 比较前 `trim + lowercase`。
- `chatgptpro` 与 `pro` 视为同一套餐。
- 归一只用于比较，不改变最终保存的上游 `plan_type` 字符串。
- 未知新套餐值不作为本任务验收范围。

Codex 快照 key 清理范围：
- 删除 `codex_5h_*`
- 删除 `codex_7d_*`
- 删除 `codex_primary_*`
- 删除 `codex_secondary_*`
- 删除 `codex_primary_over_secondary_percent`
- 删除 `codex_usage_updated_at`

不得清理：
- `credentials`
- `privacy_mode`
- 账号 quota 配置字段，如 `quota_*`
- 非 Codex `extra` 字段

错误处理：
- Token refresh 失败仍按现有逻辑失败。
- Token refresh 成功后，quota probe 失败/超时/无 header 不改变单账号成功响应结构。
- 批量刷新中 token 成功但 quota probe 失败/超时计入 success，并追加 warning。
- 后台自动刷新中 quota probe 失败/超时仅记录日志。

## Compatibility

- `/admin/openai/refresh-token` 保持现有 tokenInfo 响应，不返回 quota 快照。
- 单账号 `/admin/accounts/:id/refresh` 成功仍返回账号对象。
- `/admin/openai/accounts/:id/refresh` 成功仍返回账号对象。
- 批量刷新响应继续使用现有 `warnings` 数组。
- 非 OpenAI 平台和非 OAuth 账号刷新行为不变。

## Trade-Offs

- 手动刷新等待 30 秒 probe 能提升界面一致性，但会增加接口耗时。
- 批量刷新等待每账号 probe 会增加总耗时；保留现有并发上限 10 降低上游压力。
- 套餐变化且 probe 失败时清理旧快照会让额度窗口短暂为空，但避免旧套餐额度误导调度。

## Rollback

- 回滚新增 refresh metadata sync 调用即可恢复旧行为。
- 如果 JSONB key 删除路径出现问题，可先禁用套餐变化时的旧快照清理，保留 token refresh 和 plan_type 写回。
