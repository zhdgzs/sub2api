# Admin OAuth Reauthorization

## Scenario: OpenAI OAuth Reauthorization Metadata Sync

### 1. Scope / Trigger

- Trigger: Any change to admin OAuth reauthorization or OpenAI OAuth credential refresh behavior.
- Reason: Admin reauthorization uses `POST /api/v1/admin/accounts/:id/apply-oauth-credentials`, not the same handler as manual token refresh. OpenAI plan metadata must stay correct even when the frontend omits `plan_type`.

### 2. Signatures

- Reauthorization API: `POST /api/v1/admin/accounts/:id/apply-oauth-credentials`
- Request body:
  - `type`: `oauth` or `setup-token`
  - `credentials`: object, required
  - `extra`: object, optional
- Handler contract:
  - `AccountHandler.ApplyOAuthCredentials`
  - `syncOpenAIReauthorizedAccount(ctx, before, account, incomingCredentials)`
- Service dependencies:
  - `OpenAIOAuthService.RefreshAccountToken(ctx, account)`
  - `OpenAIOAuthService.BuildAccountCredentials(tokenInfo)`
  - `AccountUsageService.SyncOpenAIRefreshMetadata(ctx, before, after)`

### 3. Contracts

- The backend must not rely on frontend-built credentials as the only source for OpenAI `plan_type`.
- For OpenAI OAuth reauthorization, preserve existing `credentials.plan_type` and `credentials.subscription_expires_at` when the incoming payload omits or empties them.
- After applying credentials and clearing account errors, the backend must run OpenAI metadata refresh/enrichment with the newly stored token state.
- If the incoming payload does not contain a non-empty `refresh_token` but the account has an `access_token`, metadata refresh should use the current access token and must not accidentally refresh with the old stored refresh token.
- Successful enrichment must persist credentials built by `BuildAccountCredentials`, then merge any non-token existing credential keys that are not returned by enrichment.
- After OpenAI reauthorization metadata refresh, call `SyncOpenAIRefreshMetadata` so Codex quota snapshots follow the refreshed credential state.
- Non-OpenAI platforms and non-OAuth accounts must keep existing behavior.

### 4. Validation & Error Matrix

- Missing account -> HTTP 404 from `ApplyOAuthCredentials`.
- Existing account is not OAuth -> HTTP 400 reason `NOT_OAUTH`.
- OpenAI metadata refresh fails -> log warning and still return the reauthorized account; do not convert reauthorization success into failure.
- OpenAI metadata persistence fails -> log warning and return the latest successfully saved account.
- `extra` merge fails -> log error and continue; response should still use the latest account state.

### 5. Good / Base / Bad Cases

- Good: existing `plan_type=free`, reauthorization payload has no `plan_type`, enrichment returns `plus`; response and DB credentials contain `plus`.
- Good: enrichment cannot confirm plan type; existing `plan_type` remains unchanged instead of being cleared.
- Base: Anthropic/Gemini/Antigravity reauthorization continues through existing credential save and error-clear behavior.
- Bad: trusting frontend `buildCredentials(tokenInfo)` alone, because `tokenInfo.plan_type` can be absent when backend enrichment was best-effort.
- Bad: using the old stored refresh token during a reauthorization whose incoming payload only contains a new access token.

### 6. Tests Required

- Handler test for `ApplyOAuthCredentials` on OpenAI OAuth where incoming credentials omit `plan_type` and backend enrichment updates it.
- Assertion that response credentials include the new non-sensitive `plan_type`.
- Assertion that sensitive token values are still redacted and exposed only through `credentials_status`.
- Failure-path test when enrichment fails should assert old `plan_type` is preserved and HTTP success remains.
- Existing refresh-token tests should continue covering `SyncOpenAIRefreshMetadata` quota update and stale snapshot cleanup.

### 7. Wrong vs Correct

#### Wrong

```go
updatedAccount, err := h.adminService.UpdateAccount(ctx, accountID, &service.UpdateAccountInput{
    Type:        req.Type,
    Credentials: req.Credentials,
})
```

This lets a reauthorization payload without `plan_type` clear or preserve stale metadata without a server-side confirmation pass.

#### Correct

```go
credentialsToApply := preserveOpenAIReauthMetadataCredentials(existing, req.Credentials)
updatedAccount, err := h.adminService.UpdateAccount(ctx, accountID, &service.UpdateAccountInput{
    Type:        req.Type,
    Credentials: credentialsToApply,
})
updatedAccount = h.syncOpenAIReauthorizedAccount(ctx, existing, updatedAccount, credentialsToApply)
```

The backend first preserves known metadata, then confirms and overwrites it from the OpenAI token/enrichment path when possible.
