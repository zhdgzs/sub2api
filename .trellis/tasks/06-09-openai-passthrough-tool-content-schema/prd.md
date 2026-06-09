# 修复 OpenAI passthrough 工具上下文 content 转发错误

## Goal

修复 OpenAI OAuth 自动透传 `/v1/responses` 请求中，工具调用上下文 item 携带非空 `content` 导致上游返回 `array_above_max_length` 的问题。

## Requirements

- OpenAI OAuth passthrough 路径在转发前必须清理 Codex 工具调用上下文 item 上游不接受的 `content` 字段。
- 非 passthrough Codex OAuth transform 路径应复用同一类 schema 清理，避免同类错误从其他入口复现。
- WS replay 收集上游工具调用上下文时应避免保留非法 `content` 字段。
- 不删除普通 `message.content`，不删除工具输出 item 的 `output`，不影响子任务/工具执行结果。
- 不修改账号调度、failover、sticky session、token refresh、数据库或外部 API 契约。

## Acceptance Criteria

- [ ] `function_call`、`tool_call`、`local_shell_call`、`tool_search_call`、`custom_tool_call`、`mcp_tool_call`、`item_reference` 的 `content` 在 OpenAI OAuth 上游请求前被删除。
- [ ] 普通消息文本和工具输出结果保持不变。
- [ ] passthrough、非 passthrough OAuth transform、WS replay collector 均有回归测试覆盖。
- [ ] 相关 backend service 测试可在 Go 环境中通过；若本机无 Go，记录无法执行的验证命令。

## Notes

- 生产日志确认实际失败路径为 HTTP `/v1/responses`、OpenAI OAuth、`openai_passthrough=true`，最终上游错误为 `Invalid 'input[29].content': array too long`。
