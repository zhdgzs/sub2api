# Design

## Boundary

The fix stays in backend OpenAI request normalization. It does not alter account selection, retries, failover policy, sticky session bindings, token refresh, database state, or frontend behavior.

## Data Flow

OpenAI OAuth requests can reach ChatGPT internal Codex through three relevant paths:

1. OAuth passthrough `/v1/responses` uses raw JSON bytes and `normalizeOpenAIPassthroughOAuthBody`.
2. Non-passthrough OAuth requests decode to a map and call `applyCodexOAuthTransform`, which filters `input`.
3. WS replay stores tool-call context from upstream `response.output` and later replays it as full `input`.

All three paths need the same schema rule: tool-call context and `item_reference` input items must not carry `content`. The rule must not apply to `message` items or tool output items.

## Approach

- Add a shared helper in `openai_codex_transform.go` that identifies input item types whose `content` must be removed.
- Reuse it in `filterCodexInputWithOptions` for decoded-map transform paths.
- Add a raw-byte cleanup helper for passthrough normalization using `gjson` + `sjson.DeleteBytes`.
- Clean replay-collected tool-call context before storing it in `openAIWSToolCallReplayCollector`.

## Compatibility

Deleting these `content` fields is schema normalization, not context truncation. Tool-call context uses `id`, `call_id`, `name`, `arguments`, `input`, and related fields. Tool results remain in output item `output` fields and are not deleted.
