package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionAccountFieldWhitelist(t *testing.T) {
	now := time.Now().UTC()
	rate := 1.25
	prediction := 120.5
	account := &service.Account{
		ID: 8, Name: "pool-8", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "secret-token", "plan_type": "pro"},
		Extra:       map[string]any{
			"validation_url": "https://internal.example", "private": "secret-extra",
			"codex_credits_snapshot": map[string]any{
				"credits": map[string]any{
					"has_credits": true, "unlimited": false, "balance": "12345678901234567890.0123",
					"private": "secret-credit-data",
				},
				"fetched_at": int64(1770000000), "account_id": "secret-credit-account",
			},
		},
		ProxyID:     int64Pointer(99), ErrorMessage: "internal upstream failure",
		Concurrency: 5, RateMultiplier: &rate, Status: service.StatusActive, Schedulable: true,
		LastUsedAt: &now, CreatedAt: now,
	}
	item := &service.SubscriptionAccountItem{
		Account:                      account,
		Groups:                       []service.SubscriptionAccountGroup{{ID: 3, Name: "Pro", Platform: service.PlatformOpenAI}},
		CurrentOpenAIQuotaPrediction: &prediction,
		Usage: &service.UsageInfo{
			FiveHour:        &service.UsageProgress{Utilization: 42},
			Error:           "secret usage error",
			ForbiddenReason: "secret forbidden reason",
			ValidationURL:   "https://verify.example",
		},
	}

	out := userSubscriptionAccountFromService(item)
	raw, err := json.Marshal(out)

	require.NoError(t, err)
	jsonText := string(raw)
	require.Contains(t, jsonText, `"name":"pool-8"`)
	require.Contains(t, jsonText, `"rate_multiplier":1.25`)
	require.Contains(t, jsonText, `"five_hour":{"utilization":42`)
	require.Contains(t, jsonText, `"current_openai_quota_prediction":120.5`)
	require.Contains(t, jsonText, `"supports_openai_quota_history":true`)
	require.Contains(t, jsonText, `"codex_credits_snapshot":{"credits":{"has_credits":true,"unlimited":false,"balance":"12345678901234567890.0123"},"fetched_at":1770000000}`)
	require.NotContains(t, jsonText, "secret-credit-data")
	require.NotContains(t, jsonText, "secret-credit-account")
	require.NotContains(t, jsonText, "secret-token")
	require.NotContains(t, jsonText, "secret-extra")
	require.NotContains(t, jsonText, "internal upstream failure")
	require.NotContains(t, jsonText, "verify.example")
	require.NotContains(t, jsonText, "secret usage error")
	require.NotContains(t, jsonText, "secret forbidden reason")
	require.NotContains(t, jsonText, "proxy_id")
	require.NotContains(t, jsonText, "credentials")
	require.NotContains(t, jsonText, "extra")
}

func TestUserSubscriptionAccountCodexCreditsSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		snapshot string
		want     string
	}{
		{
			name:     "decimal balance preserves precision",
			snapshot: `{"credits":{"has_credits":true,"unlimited":false,"balance":"12345678901234567890.0123"},"fetched_at":1770000000}`,
			want:     `{"credits":{"has_credits":true,"unlimited":false,"balance":"12345678901234567890.0123"},"fetched_at":1770000000}`,
		},
		{
			name:     "zero balance",
			snapshot: `{"credits":{"has_credits":false,"unlimited":false,"balance":"0"},"fetched_at":1770000000}`,
			want:     `{"credits":{"has_credits":false,"unlimited":false,"balance":"0"},"fetched_at":1770000000}`,
		},
		{
			name:     "unlimited balance",
			snapshot: `{"credits":{"has_credits":false,"unlimited":true,"balance":null},"fetched_at":1770000000}`,
			want:     `{"credits":{"has_credits":false,"unlimited":true,"balance":null},"fetched_at":1770000000}`,
		},
		{
			name:     "balance not disclosed",
			snapshot: `{"credits":{"has_credits":true,"unlimited":false},"fetched_at":1770000000}`,
			want:     `{"credits":{"has_credits":true,"unlimited":false,"balance":null},"fetched_at":1770000000}`,
		},
		{name: "null snapshot", snapshot: `null`},
		{name: "null credits", snapshot: `{"credits":null,"fetched_at":1770000000}`},
		{name: "missing credits", snapshot: `{}`},
		{name: "missing credit flags", snapshot: `{"credits":{"balance":"10"}}`},
		{name: "wrong credit flags", snapshot: `{"credits":{"has_credits":"true","unlimited":false}}`},
		{name: "numeric balance", snapshot: `{"credits":{"has_credits":true,"unlimited":false,"balance":10}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var snapshot any
			require.NoError(t, json.Unmarshal([]byte(tt.snapshot), &snapshot))
			account := &service.Account{
				Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
				Extra: map[string]any{"codex_credits_snapshot": snapshot},
			}
			out := userSubscriptionAccountCodexCreditsFromService(account)
			if tt.want == "" {
				require.Nil(t, out)
				return
			}
			raw, err := json.Marshal(out)
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(raw))
		})
	}
}

func TestUserSubscriptionAccountCodexCreditsOnlyForOpenAIOAuth(t *testing.T) {
	for _, account := range []*service.Account{
		nil,
		{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
		{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth},
	} {
		if account != nil && !account.IsOpenAIOAuth() {
			account.Extra = map[string]any{
				"codex_credits_snapshot": map[string]any{
					"credits": map[string]any{"has_credits": true, "unlimited": false, "balance": "10"},
				},
			}
		}
		require.Nil(t, userSubscriptionAccountCodexCreditsFromService(account))
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
