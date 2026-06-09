# Implementation Plan

## Checklist

- [x] Add a shared helper for input item types that disallow `content`.
- [x] Update decoded-map Codex input filtering to delete `content` on those item types.
- [x] Update passthrough OAuth raw-body normalization to delete `input[n].content` for those item types.
- [x] Update WS replay collector to sanitize collected tool-call context.
- [x] Add regression tests for passthrough, transform, and replay collector behavior.
- [x] Run focused backend service tests or report missing Go toolchain.

## Validation

```bash
go test ./backend/internal/service -run 'Test.*(Passthrough|CodexOAuthTransform|ToolCallReplay|InputContent)'
go test ./backend/internal/service -run 'TestOpenAIGatewayService_Forward.*Passthrough|TestApplyCodexOAuthTransform'
```

Current environment result: both `go test` commands are blocked by `/bin/bash: line 1: go: command not found`.

## Review Notes

- Do not log request body contents.
- Do not remove `message.content`.
- Do not remove `function_call_output.output`, `custom_tool_call_output.output`, `mcp_tool_call_output.output`, or `tool_search_output.output`.
