# 刷新令牌同步账号套餐和额度 - Implementation Plan

## Checklist

1. 增加共享的 OpenAI refresh metadata 同步逻辑：
   - plan_type 规范化比较 helper。
   - Codex snapshot key 列表和内存清理 helper。
   - 30 秒 probe 上下文。
   - probe 成功合并 extra，失败时按 plan 变化清理旧快照。
2. 在 repository 增加 Codex snapshot key 删除能力：
   - 使用 JSONB `-` 删除指定 key。
   - 保持 scheduler-neutral 语义并同步 fresh scheduler cache。
3. 接入管理端单账号刷新：
   - `AccountHandler.refreshSingleAccount` OpenAI 分支刷新 credentials 后等待 metadata sync。
   - 保持单账号响应为 `Account`。
4. 接入管理端批量刷新：
   - 保持并发上限 10。
   - token 成功但 quota sync warning 时计入 success，并追加 warning。
5. 接入 OpenAI 专用账号刷新接口：
   - `/admin/openai/accounts/:id/refresh` 同样等待 metadata sync。
6. 接入后台自动刷新：
   - OpenAI OAuth refresh 成功后执行 best-effort metadata sync。
   - 不处理无 refresh_token 账号。
   - 不因 sync 失败改变 refresh 成功结果。
7. 补充测试：
   - plan_type free -> plus / plus -> free 写回。
   - `chatgptpro` vs `pro` 不触发快照清理。
   - probe 成功写入 `codex_5h_*` / `codex_7d_*`。
   - probe 失败且 plan 变化清理旧快照。
   - probe 失败且 plan 未变化保留旧快照。
   - 批量 refresh warning 计入 success。
   - 后台自动刷新触发 best-effort sync，不影响 refresh 成功。

## Validation Commands

- `go test -tags unit ./internal/service -run 'Test(OpenAI|TokenRefresh|BuildCodex|Refresh)'`
- `go test -tags unit ./internal/handler/admin -run 'Test.*Refresh'`
- `go test ./internal/repository -run 'Test.*Extra|Test.*Codex'`

如果聚焦命令因 build tags 或包依赖不匹配失败，再按实际失败包收窄到具体测试文件所在 package。

## Risk Points

- `UpdateExtra` 不能删除 key，清理逻辑必须走新删除方法或原子 SQL。
- 手动 refresh 返回账号对象必须包含 sync 后的 extra；不要改 API wrapper。
- 后台 refresh 中 `OAuthRefreshAPI.RefreshIfNeeded` 返回的 account 可能是刷新前对象，credentials 持久化后需确保 probe 使用新 access token。
- 批量 refresh 的 warning 不应把账号计为 failed。

## Rollback Points

- 如果 probe 接入导致刷新慢或失败，可先保留 plan_type 写回，禁用 quota probe 调用。
- 如果删除 extra key SQL 有风险，可临时关闭套餐变化时清理旧快照，保留 probe 成功写入路径。
