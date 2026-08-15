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
	account := &service.Account{
		ID: 8, Name: "pool-8", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "secret-token"},
		Extra:       map[string]any{"validation_url": "https://internal.example", "private": "secret-extra"},
		ProxyID:     int64Pointer(99), ErrorMessage: "internal upstream failure",
		Concurrency: 5, RateMultiplier: &rate, Status: service.StatusActive, Schedulable: true,
		LastUsedAt: &now, CreatedAt: now,
	}
	item := &service.SubscriptionAccountItem{
		Account: account,
		Groups:  []service.SubscriptionAccountGroup{{ID: 3, Name: "Pro", Platform: service.PlatformOpenAI}},
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

func int64Pointer(value int64) *int64 {
	return &value
}
