# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

<!-- Patterns that must always be used -->

### OpenAI Responses OAuth Input Schema Normalization

When forwarding OpenAI OAuth `/v1/responses` payloads to ChatGPT internal Codex endpoints, normalize unsupported input-item fields at every request construction boundary:

- `function_call`, `tool_call`, `local_shell_call`, `tool_search_call`, `custom_tool_call`, `mcp_tool_call`, and `item_reference` input items must not carry `content`.
- Do not delete `message.content`.
- Do not delete tool output `output` fields such as `function_call_output.output`, `custom_tool_call_output.output`, `mcp_tool_call_output.output`, or `tool_search_output.output`.
- Reuse the shared helper that owns this type table; do not duplicate local switch statements in passthrough, transform, or replay code.

This prevents upstream schema failures such as `Invalid 'input[n].content': array too long` without losing message context or tool/subtask results.

---

## Testing Requirements

<!-- What level of testing is expected -->

(To be filled by the team)

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)
