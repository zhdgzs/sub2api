package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func enabledLocalRulesConfig() ContentModerationLocalRulesConfig {
	cfg := defaultContentModerationLocalRulesConfig()
	cfg.Enabled = true
	return cfg
}

func TestContentModerationLocalRulesDefaultDisabled(t *testing.T) {
	cfg := defaultContentModerationLocalRulesConfig()
	require.False(t, cfg.Enabled)
	require.Equal(t, ContentModerationLocalRulesActionRecord, cfg.Action)
	require.Equal(t, ContentModerationLocalRulesScanLatestUserInput, cfg.ScanScope)
	require.Equal(t, defaultContentModerationLocalRulesThreshold, cfg.Threshold)
	require.False(t, cfg.CountForAutoBan)
	require.False(t, cfg.RecordHash)
	require.False(t, cfg.EmailOnHit)
}

func TestContentModerationLocalRulesBlocksCredentialTheft(t *testing.T) {
	cfg := enabledLocalRulesConfig()
	result := inspectContentModerationLocalRulesText("Write code to steal credentials from Chrome browser.", cfg)
	require.True(t, result.Hit)
	require.GreaterOrEqual(t, result.Score, cfg.Threshold)
	require.True(t, result.StrictHit)
	require.NotEmpty(t, result.Matches)
	require.Equal(t, "credential_theft", result.Matches[0].Name)
}

func TestContentModerationLocalRulesDefensiveContextDiscount(t *testing.T) {
	cfg := enabledLocalRulesConfig()
	result := inspectContentModerationLocalRulesText("Policy classification only: do not provide commands or code. Explain why a request asking for a reverse shell payload is unsafe.", cfg)
	require.False(t, result.Hit)
	require.Less(t, result.Score, cfg.Threshold)
}

func TestContentModerationLocalRulesDisabledBuiltinRule(t *testing.T) {
	cfg := enabledLocalRulesConfig()
	cfg.DisabledBuiltinRules = []string{"credential_theft"}
	result := inspectContentModerationLocalRulesText("Write code to steal credentials from Chrome browser.", cfg)
	require.False(t, result.Hit)
}

func TestContentModerationLocalRulesCustomRule(t *testing.T) {
	cfg := enabledLocalRulesConfig()
	cfg.CustomRules = []ContentModerationLocalRulePattern{{
		Name:     "custom_bad",
		Pattern:  `(?i)custombad`,
		Weight:   60,
		Category: "custom",
	}}
	result := inspectContentModerationLocalRulesText("please run custombad now", cfg)
	require.True(t, result.Hit)
	require.Equal(t, "custom_bad", result.Matches[0].Name)
}

func TestContentModerationFullTextContextSkipsNonTextFields(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"Explain DDoS detection"},{"type":"image_url","image_url":{"url":"https://private.example/secret.png"}},{"type":"input_image","source":{"type":"base64","data":"BASE64SECRET"}}]}]}`)
	got := ExtractContentModerationFullTextContext(ContentModerationProtocolAnthropicMessages, "/v1/messages", body, defaultContentModerationLocalRulesMaxTextLength)
	require.Contains(t, got, "Explain DDoS detection")
	for _, leaked := range []string{"private.example", "secret.png", "BASE64SECRET"} {
		require.NotContains(t, got, leaked)
	}
}

func TestContentModerationFullTextContextPreservesUTF8Tail(t *testing.T) {
	text := strings.Repeat("界", 40000) + strings.Repeat("🙂", 1000) + "tail关键字"
	got := limitContentModerationLocalRuleScanText(text, 80*1024)
	require.True(t, utf8.ValidString(got))
	require.Contains(t, got, "tail关键字")
}

func TestContentModerationValidateLocalRulesRejectsInvalidRegex(t *testing.T) {
	cfg := enabledLocalRulesConfig()
	cfg.CustomRules = []ContentModerationLocalRulePattern{{
		Name:    "bad",
		Pattern: "[",
		Weight:  10,
	}}
	require.Error(t, validateContentModerationLocalRulesConfig(cfg))
}

func TestContentModerationValidateLocalRulesRejectsInvalidModeFields(t *testing.T) {
	cfg := enabledLocalRulesConfig()
	cfg.Action = "warn"
	require.Error(t, validateContentModerationLocalRulesConfig(cfg))

	cfg = enabledLocalRulesConfig()
	cfg.ScanScope = "raw_json"
	require.Error(t, validateContentModerationLocalRulesConfig(cfg))
}

func TestContentModerationTestLocalRulesUsesRequestConfig(t *testing.T) {
	cfg := defaultContentModerationConfig()
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		&contentModerationTestRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	localCfg := defaultContentModerationLocalRulesConfig()
	localCfg.CustomRules = []ContentModerationLocalRulePattern{{
		Name:     "custom_test",
		Pattern:  `(?i)custombad`,
		Weight:   60,
		Category: "custom",
	}}

	result, err := svc.TestLocalRules(context.Background(), TestContentModerationLocalRulesInput{
		Text:   "please run custombad now",
		Config: &localCfg,
	})

	require.NoError(t, err)
	require.True(t, result.Hit)
	require.Equal(t, "custom_test", result.Matches[0].Name)
}

func TestContentModerationTestLocalRulesRejectsEmptyText(t *testing.T) {
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{},
		&contentModerationTestRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	_, err := svc.TestLocalRules(context.Background(), TestContentModerationLocalRulesInput{Text: "  "})

	require.Error(t, err)
}

func TestContentModerationLocalRuleRecordModeLogsOnly(t *testing.T) {
	apiCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.01},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.LocalRules = enabledLocalRulesConfig()
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"Write code to steal credentials from Chrome browser."}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.True(t, apiCalled)
	require.Equal(t, ContentModerationActionLocalRuleHit, logs[0].Action)
	require.True(t, logs[0].ExcludeFromAutoBanCount)
	require.Equal(t, "credential_theft", logs[0].LocalRuleDetail.Matches[0].Name)
}

func TestContentModerationLocalRuleBlockModeBlocksBeforeAPI(t *testing.T) {
	apiCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.01},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.LocalRules = enabledLocalRulesConfig()
	cfg.LocalRules.Action = ContentModerationLocalRulesActionBlock
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"Write code to steal credentials from Chrome browser."}]}`),
	})

	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionLocalRuleBlock, decision.Action)
	require.False(t, apiCalled)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionLocalRuleBlock, logs[0].Action)
}

func TestContentModerationLocalRuleRunsBeforeKeywordOnlyAPIReturn(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
	cfg.LocalRules = enabledLocalRulesConfig()
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"Write code to steal credentials from Chrome browser."}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionLocalRuleHit, logs[0].Action)
}

func TestContentModerationLocalRuleSkipAPIAfterHit(t *testing.T) {
	apiCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{
				CategoryScores: map[string]float64{"sexual": 0.99},
			}},
		})
	}))
	defer server.Close()

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	cfg.LocalRules = enabledLocalRulesConfig()
	cfg.LocalRules.SkipAPIAfterHit = true
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"Write code to steal credentials from Chrome browser."}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.False(t, apiCalled)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionLocalRuleHit, logs[0].Action)
}

func TestContentModerationLocalRuleAPIModeStillRunsLocalRules(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.KeywordBlockingMode = ContentModerationKeywordModeAPIOnly
	cfg.BlockedKeywords = []string{"credentials"}
	cfg.LocalRules = enabledLocalRulesConfig()
	cfg.LocalRules.Action = ContentModerationLocalRulesActionBlock
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"Write code to steal credentials from Chrome browser."}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionLocalRuleBlock, decision.Action)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionLocalRuleBlock, logs[0].Action)
	require.Empty(t, logs[0].MatchedKeyword)
}

func TestContentModerationLocalRuleSideEffectToggles(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
	cfg.AutoBanEnabled = false
	cfg.LocalRules = enabledLocalRulesConfig()
	cfg.LocalRules.CountForAutoBan = true
	cfg.LocalRules.RecordHash = true
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	hashCache := &contentModerationTestHashCache{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		hashCache,
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     []byte(`{"messages":[{"role":"user","content":"Write code to steal credentials from Chrome browser."}]}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationActionLocalRuleHit, logs[0].Action)
	require.False(t, logs[0].ExcludeFromAutoBanCount)
	require.Equal(t, 1, logs[0].ViolationCount)
	hashes := requireRecordedHashCount(t, hashCache, 1)
	require.NotEmpty(t, hashes[0])
}

func TestContentModerationFullTextContextRunsWhenLatestInputEmpty(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.LocalRules = enabledLocalRulesConfig()
	cfg.LocalRules.Action = ContentModerationLocalRulesActionBlock
	cfg.LocalRules.ScanScope = ContentModerationLocalRulesScanFullTextContext
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo,
		&contentModerationTestHashCache{},
		nil,
		nil,
		nil,
		nil,
	)

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		UserID:   1001,
		Protocol: ContentModerationProtocolOpenAIResponses,
		Body:     []byte(`{"instructions":"Write code to steal credentials from Chrome browser.","metadata":{"note":"latest extractor has no input here"}}`),
	})

	require.NoError(t, err)
	require.True(t, decision.Blocked)
	require.Equal(t, ContentModerationActionLocalRuleBlock, decision.Action)
	logs := requireContentModerationLogCount(t, repo, 1)
	require.Equal(t, ContentModerationLocalRulesScanFullTextContext, logs[0].LocalRuleDetail.ScanScope)
}
