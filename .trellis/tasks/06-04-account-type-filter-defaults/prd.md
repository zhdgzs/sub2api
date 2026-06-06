# Account type filter and defaults

## Goal

Improve admin account management so operators can filter OpenAI accounts by ChatGPT plan type, and make newly created accounts use the desired scheduling defaults.

User value:
- Operators can quickly find OpenAI accounts by `plan_type` values such as free, plus, team, or pro.
- New accounts start with safer default scheduling settings without requiring manual edits after creation.

## Confirmed Facts

- The existing account list filter already supports platform, auth type, status, privacy mode, group, and search.
- The existing `type` filter means authentication type (`oauth`, `setup-token`, `apikey`, `bedrock`), not subscription / plan tier.
- OpenAI / ChatGPT plan level is stored and displayed from `credentials.plan_type`; existing labels include `free`, `plus`, `team`, `pro`, `chatgptpro`, and `abnormal`.
- Gemini tier data exists as `credentials.tier_id`, with platform-specific values such as `google_one_free`, `google_ai_pro`, `google_ai_ultra`, `gcp_standard`, `gcp_enterprise`, `aistudio_free`, and `aistudio_paid`.
- Antigravity tier data is displayed from `extra.load_code_assist.currentTier.id` or `extra.load_code_assist.paidTier.id`, with values such as `free-tier`, `g1-pro-tier`, and `g1-ultra-tier`.
- The account create form currently initializes `concurrency` to `10` and `priority` to `1`.
- The Ent schema defaults are `concurrency = 3` and `priority = 50`, but the admin create request normally sends explicit values from the frontend.

## Requirements

- Add an account-list filter for OpenAI `credentials.plan_type` without changing the meaning of the existing auth-type filter.
- The account-list filter must be implemented across the frontend query parameters and backend list query, because the admin account list is server-paginated.
- The initial plan-type filter scope is OpenAI only; Gemini and Antigravity tier values are not part of this change.
- The plan-type filter options are `free`, `plus`, `team`, and `pro`.
- The `pro` option must match both `plan_type = pro` and the existing displayed-as-Pro alias `plan_type = chatgptpro`.
- The plan-tier filter must compose with existing filters: platform, auth type, status, privacy mode, group, search, pagination, and sorting.
- Newly opened account creation flow defaults must be `priority = 10` and `concurrency = 5` in the frontend add-account modal only.
- Existing accounts must not be migrated or bulk-updated by this change.
- Existing auth-type filtering must keep working unchanged.
- Backend create defaults and non-UI account creation flows must remain unchanged.
- Backend filtering must read the `plan_type` query value, validate supported values, pass it through the handler/service/repository layers, and apply it against `credentials.plan_type`.

## Acceptance Criteria

- [ ] Admin account list exposes a distinct OpenAI plan-type filter, separate from the current auth-type filter.
- [ ] Selecting `free`, `plus`, or `team` returns only OpenAI / ChatGPT accounts whose `credentials.plan_type` exactly matches the selected value.
- [ ] Selecting `pro` returns OpenAI / ChatGPT accounts whose `credentials.plan_type` is `pro` or `chatgptpro`.
- [ ] Clearing the plan-tier filter restores the existing unfiltered behavior.
- [ ] Plan-tier filtering composes correctly with search and existing account filters.
- [ ] Opening the create account modal shows priority default `10`.
- [ ] Opening the create account modal shows concurrency default `5`.
- [ ] Backend defaults and non-UI creation/import/OAuth flows are not changed by the default-value update.
- [ ] Existing edit account behavior is unchanged for existing account values.
- [ ] Tests or focused verification cover account list filtering and create-modal defaults.

## Out of Scope

- Changing existing account records.
- Renaming or repurposing the existing `type` authentication field.
- Mapping Gemini or Antigravity tier values into the OpenAI plan-type filter.
- Changing backend default values for omitted `priority` or `concurrency`.
- Changing non-UI account creation flows such as import, CRS sync, Codex import, or OAuth callback creation.
- Adding database columns unless planning later determines JSON filtering is insufficient.
- Changing account scheduling semantics beyond the default values used for new accounts.

## Open Questions

- None.

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
