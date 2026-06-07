# 刷新令牌同步账号套餐和额度

## Goal

扩展 OpenAI/Codex OAuth 账号的刷新令牌能力，使账号在刷新 access token 后同步最新套餐状态和 Codex 用量窗口快照。

用户价值：
- 原本为 free 的账号开通 Plus 后，刷新令牌即可把账号切换到新的 `plan_type`。
- 原本为 Plus/Pro 等付费账号过期回到 free 后，刷新令牌即可把账号状态回落，避免调度和筛选继续依赖陈旧套餐。
- 刷新令牌后同步 5h / 7d 用量百分比和重置时间，避免额度窗口展示、自动暂停、调度可用性继续依赖旧快照。

## Confirmed Facts

- OpenAI OAuth 刷新由 `OpenAIOAuthService.RefreshAccountToken` 执行，内部会调用 `RefreshTokenWithClientID`，然后通过 `enrichTokenInfo` 查询 `chatgpt.com/backend-api/accounts/check` 和 `/backend-api/subscriptions`。
- `enrichTokenInfo` 当前可以补全 `plan_type`、`subscription_expires_at`、email，并尝试设置 OpenAI privacy。
- `BuildAccountCredentials` 会把非空 `PlanType` 写入 credentials 的 `plan_type`；本任务需要验证并锁定手动刷新、批量刷新、后台自动刷新都能可靠覆盖 free/paid 双向变化。
- 后台自动刷新走 `TokenRefreshService` + `OAuthRefreshAPI.RefreshIfNeeded`，成功后只做 credentials 持久化、缓存失效、调度缓存同步和 privacy 检查。
- 管理端账号列表单账号/批量“刷新令牌”走 `AccountHandler.refreshSingleAccount`，也只更新 credentials、失效 token 缓存和设置 privacy。
- OpenAI 专用 `/admin/openai/accounts/:id/refresh` 也直接刷新 credentials，不更新 Codex 用量窗口 `extra`。
- 新增/验证 refresh token 的 `/admin/openai/refresh-token` 只返回 `tokenInfo` 给前端创建账号，不更新已有账号。
- Codex quota 快照已有统一字段和转换逻辑：`ParseCodexRateLimitHeaders` + `buildCodexUsageExtraUpdates` 会写入 `codex_5h_used_percent`、`codex_5h_reset_after_seconds`、`codex_5h_reset_at`、`codex_7d_used_percent`、`codex_7d_reset_after_seconds`、`codex_7d_reset_at`、`codex_usage_updated_at` 等 `extra` 字段。
- `AccountUsageService` 已有 OpenAI Codex probe，会用当前 access token 发一次轻量请求获取 Codex rate-limit headers，并通过 `UpdateExtra` 持久化 quota 快照。
- `UpdateExtra` 对 `codex_*` 快照字段有 scheduler-neutral 处理，但 exhausted snapshot 仍会同步 fresh scheduler cache，避免长期旧快照影响调度。
- 429 响应体里已有 `persistOpenAI429PlanType`，会把 `usage_limit_reached/rate_limit_exceeded` 的 `plan_type` 回写 credentials，但这只发生在 429 路径。
- 已确认范围：后台自动刷新也必须覆盖本任务；OpenAI OAuth token refresh 成功后允许额外执行一次 best-effort Codex quota probe。
- 已确认交互：管理端手动刷新接口需要等待 Codex quota probe 完成后再返回，确保返回账号尽量包含最新额度窗口；后台自动刷新无接口返回，但刷新成功后也同步执行短超时 best-effort probe，probe 失败不影响 token refresh 成功。
- 已确认超时：Codex quota probe 等待上限为 30 秒；超时只影响 quota 快照同步，不影响 token refresh 成功落库。
- 已确认批量行为：批量刷新也必须逐账号等待 Codex quota probe；现有并发上限可保持为 10。单个账号 token refresh 成功但 quota probe 失败/超时时，该账号计为刷新成功，并在批量结果中返回 warning。
- 已确认不覆盖新增/验证 RT：`/admin/openai/refresh-token` 只用于验证 refresh token 并返回 tokenInfo 给创建账号流程；本任务不要求该接口返回 Codex quota 快照，也不改前端 RT 创建账号流程。
- 已确认单账号响应：单账号刷新中 token refresh 成功但 Codex quota probe 失败/超时时，不改变响应结构、不返回 warning；接口仍返回 `Account`，quota 同步问题只记录日志，返回账号保留既有 `extra`。
- 已确认套餐变化时的旧快照处理：当 `plan_type` 已确认发生变化但 Codex quota probe 失败、超时或无可用 header 时，必须清理旧 `extra.codex_*` quota 快照，避免旧套餐额度继续影响展示和调度。
- `UpdateExtra` 当前使用 JSONB 合并语义，不支持删除 key；清理旧 Codex 快照需要实现明确的 JSONB key 删除路径或等价能力。
- 已确认 `plan_type` 变化判断规则：比较前需要做大小写/空格归一，并把 `chatgptpro` 与 `pro` 视为同一套餐；归一仅用于判断是否变化，不改变最终写入的上游 `plan_type` 原值。
- 已确认未知套餐值不纳入本轮讨论和验收；本任务只覆盖当前已知 `free`、`plus`、`team`、`pro` 以及 `chatgptpro` 别名的状态切换。
- 已确认无 refresh token 的手动刷新语义：OpenAI OAuth 账号没有 `refresh_token` 但仍有 `access_token` 时，管理端手动刷新也必须尝试同步 `plan_type` 和 Codex quota；后台自动刷新仍保持现有行为，跳过没有 `refresh_token` 的账号。
- 已确认无 refresh token 且 access token 不可用时的手动刷新语义：不扩大现有失败语义；只要现有刷新路径能返回账号，plan/quota 同步失败仍按 best-effort 处理，不把本任务变成账号有效性校验。

## Requirements

- OpenAI OAuth 账号刷新令牌成功后，必须同步最新 `credentials.plan_type`，支持 free -> plus/pro/team 和 plus/pro/team -> free 的双向变化。
- OpenAI OAuth 账号刷新令牌成功后，必须同步 Codex 5h / 7d quota 快照到 `extra`，包括 used percent、reset-after seconds、reset-at 和 updated-at。
- 覆盖范围包括后台自动刷新、管理端单账号手动刷新、管理端批量刷新，以及 OpenAI 专用账号刷新接口。
- 管理端手动刷新中的 OpenAI OAuth 账号即使没有 `refresh_token`，只要存在 `access_token`，也应执行套餐 enrichment 和 Codex quota probe。
- 后台自动刷新不应新增无 `refresh_token` 账号的扫描/刷新行为。
- 无 `refresh_token` 且 `access_token` 不可用时，不新增强制失败逻辑；不可用状态仍由现有测试账号、请求路径或后续 token 使用暴露。
- 套餐同步必须优先使用当前 access token 能确认的上游账号信息；当上游明确返回 free 时必须覆盖旧的 plus/pro/team 值。
- 当上游账号信息查询失败或没有返回可确认套餐时，不应把旧套餐误清空为 free；应保留现有值并记录 best-effort 失败日志。
- Quota 同步应在 token refresh 成功后额外执行 best-effort Codex quota probe，并复用现有 Codex header 解析与 extra 更新逻辑，避免重复维护字段映射。
- 管理端手动刷新接口必须等待 Codex quota probe 完成，并在返回账号对象前合并已获取的 `extra.codex_*` 更新。
- 管理端批量刷新必须等待每个成功 OpenAI OAuth 账号的 Codex quota probe 完成或 30 秒超时，并在结果中区分 token refresh 失败与 quota probe warning。
- 后台自动刷新必须在 token refresh 成功后尝试执行 Codex quota probe，但 probe 失败、30 秒超时或无 header 时只记录日志，不改变 token refresh 成功结果。
- Quota 同步失败不应导致 token refresh 整体失败；刷新 access token 仍应成功落库。
- 刷新后返回给前端/调度缓存的账号对象应尽量包含最新 credentials 和 extra 快照，避免页面刷新后仍展示旧状态。
- 单账号刷新接口保持现有响应契约：成功时直接返回账号对象，不新增 wrapper 或 warning 字段。
- 当 `plan_type` 确认变化且 Codex quota probe 未能写入新快照时，必须删除旧运行态 Codex 快照字段，包括 `codex_5h_*`、`codex_7d_*`、`codex_primary_*`、`codex_secondary_*`、`codex_primary_over_secondary_percent`、`codex_usage_updated_at`。
- 清理旧 Codex 快照不得删除账号配置字段、凭证字段、privacy 字段、quota 配置字段或非 OpenAI 平台字段。
- 判断 `plan_type` 是否变化时必须使用规范化比较：trim、lowercase，并将 `chatgptpro` 归一为 `pro`。
- 规范化比较不得改变 credentials 中最终保存的上游 `plan_type` 字符串；保存仍以 refresh/enrichment 返回值为准。
- 未知新 `plan_type` 值不作为本任务的显式需求或验收条件。
- 非 OpenAI 平台刷新令牌行为保持不变。
- 非 OAuth 账号刷新仍按现有逻辑拒绝。
- 新增/验证 refresh token 的 `/admin/openai/refresh-token` 保持现有响应语义，不返回 quota 快照。

## Acceptance Criteria

- [ ] 对已有 OpenAI OAuth 账号执行单账号刷新后，`credentials.plan_type` 从 `free` 变为上游最新付费套餐值。
- [ ] 对已有 OpenAI OAuth 账号执行单账号刷新后，`credentials.plan_type` 从付费套餐值变为上游最新 `free`。
- [ ] 后台自动刷新 OpenAI OAuth token 后，也能同步同样的 `plan_type` 变化。
- [ ] 后台自动刷新 OpenAI OAuth token 成功后，会额外尝试 Codex quota probe。
- [ ] 批量刷新 OpenAI OAuth 账号时，每个成功账号都尝试同步 `plan_type` 和 Codex quota 快照。
- [ ] 批量刷新中某账号 token refresh 成功但 Codex quota probe 失败/超时时，该账号计入 success，并在 `warnings` 中包含该账号的 quota probe warning。
- [ ] 管理端单账号刷新成功响应中的账号对象包含已成功 probe 到的最新 `extra.codex_*` 字段。
- [ ] OpenAI 专用账号刷新接口成功响应中的账号对象包含已成功 probe 到的最新 `extra.codex_*` 字段。
- [ ] 无 `refresh_token` 但有 `access_token` 的 OpenAI OAuth 账号执行管理端手动刷新时，也会尝试同步 `credentials.plan_type` 和 `extra.codex_*`。
- [ ] 后台自动刷新仍不会处理无 `refresh_token` 的 OpenAI OAuth 账号。
- [ ] 无 `refresh_token` 且 `access_token` 不可用时，管理端手动刷新不因本任务新增的 plan/quota 同步失败而改变现有响应契约。
- [ ] 刷新成功且 Codex probe 返回 rate-limit headers 时，`extra` 包含更新后的 `codex_5h_*`、`codex_7d_*` 和 `codex_usage_updated_at`。
- [ ] Codex quota probe 失败、无 header、429 或网络错误时，不影响 token credentials 更新成功。
- [ ] Codex quota probe 超过 30 秒时，刷新令牌仍返回/记录成功；若 `plan_type` 未变化则 quota 快照保持原值，若 `plan_type` 已确认变化则清理旧快照。
- [ ] 单账号刷新中 Codex quota probe 失败/超时时，响应仍是账号对象，前端无需适配新的响应结构。
- [ ] `plan_type` 发生变化且 Codex quota probe 失败/超时时，旧 `extra.codex_*` 快照字段被清理。
- [ ] `plan_type` 未变化且 Codex quota probe 失败/超时时，旧 `extra.codex_*` 快照保持原值。
- [ ] 旧值 `chatgptpro`、新值 `pro` 不会被误判为套餐变化，也不会触发旧 Codex 快照清理。
- [ ] 清理旧 Codex 快照不影响 `privacy_mode`、账号 quota 配置、credentials 或非 Codex extra 字段。
- [ ] 上游账号信息查询失败或无明确套餐时，不把现有 `plan_type` 清空或误降级。
- [ ] 非 OpenAI 平台刷新令牌测试/行为保持不变。
- [ ] 单元测试覆盖 OpenAI plan_type 覆盖、空值保留、quota 快照写入和 quota probe 失败不阻断刷新。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.

## Out of Scope

- 新增数据库列；继续使用 `credentials` 和 `extra` JSONB。
- 修改非 OpenAI 平台的刷新令牌语义。
- 修改 Codex quota 快照字段命名或展示格式。
- 改变 429 限流状态持久化策略。
- 大批量迁移历史账号数据。
- 改造新增账号的 refresh token 验证/创建流程。
- 设计未知新 OpenAI 套餐值的产品语义。

## Open Questions

- None.
