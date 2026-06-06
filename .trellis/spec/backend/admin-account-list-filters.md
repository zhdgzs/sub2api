# Admin Account List Filters

## Scenario: Server-Paginated Account Filters

### 1. Scope / Trigger

- Trigger: Any new or changed admin account-list filter used by `/api/v1/admin/accounts`, export, or filter-target bulk update.
- Reason: The admin account list is server-paginated. Frontend-only filtering only affects the current page and makes totals, export, and bulk actions inconsistent.

### 2. Signatures

- HTTP list: `GET /api/v1/admin/accounts?...`
- HTTP export: `GET /api/v1/admin/accounts/data?...`
- HTTP bulk update by filters: `POST /api/v1/admin/accounts/bulk-update`
- Handler/service signature:
  `ListAccounts(ctx, page, pageSize, platform, accountType, planType, status, search, groupID, privacyMode, sortBy, sortOrder)`
- Repository signature:
  `ListWithFilters(ctx, params, platform, accountType, planType, status, search, groupID, privacyMode)`

### 3. Contracts

- `platform`: optional string; filters `accounts.platform`.
- `type`: optional string; filters auth type only, such as `oauth` or `apikey`.
- `plan_type`: optional OpenAI-only plan filter. Supported values are `free`, `plus`, `team`, and `pro`.
- `status`, `group`, `privacy_mode`, `search`, `sort_by`, and `sort_order` must continue composing with any new filter.
- When `plan_type` is present, repository filtering must constrain the account platform to OpenAI and inspect `credentials.plan_type`.
- `plan_type=pro` must match both `credentials.plan_type = "pro"` and `"chatgptpro"`.
- The list ETag payload must include any filter that changes the returned rows.

### 4. Validation & Error Matrix

- Missing or empty `plan_type` -> no plan-type filtering.
- `free`, `plus`, `team`, `pro` -> accepted after trim/lowercase normalization.
- Any other `plan_type` -> HTTP 400 with reason `INVALID_PLAN_TYPE_FILTER`.
- Invalid numeric `group` -> HTTP 400 with reason `INVALID_GROUP_FILTER`.

### 5. Good / Base / Bad Cases

- Good: `?platform=openai&type=oauth&plan_type=pro&status=active` returns only matching OpenAI OAuth accounts with `pro` or `chatgptpro`.
- Base: no `plan_type` preserves existing platform/type/status/search behavior.
- Bad: frontend filters the already-returned page locally while backend still returns unfiltered totals and pages.

### 6. Tests Required

- Handler unit tests assert valid `plan_type` reaches `AdminService.ListAccounts`.
- Handler unit tests assert invalid `plan_type` returns `INVALID_PLAN_TYPE_FILTER`.
- Service unit tests assert `planType` is passed into `AccountRepository.ListWithFilters`.
- Repository integration tests assert exact values and aliases against `credentials.plan_type`.
- Export tests assert `plan_type` reaches the same list path.
- Bulk-update tests assert filter-target payloads preserve `plan_type`.
- Frontend API/component tests assert the query parameter and filter payload use `plan_type`, not `type`.

### 7. Wrong vs Correct

#### Wrong

```typescript
// Only filters whatever page has already been loaded.
const filtered = accounts.value.filter(account => account.credentials?.plan_type === planType)
```

#### Correct

```typescript
await adminAPI.accounts.list(page, pageSize, {
  platform: 'openai',
  type: 'oauth',
  plan_type: 'pro',
})
```

```go
accounts, result, err := repo.ListWithFilters(
    ctx, params, platform, accountType, planType, status, search, groupID, privacyMode,
)
```
