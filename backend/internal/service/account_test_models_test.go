package service

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestFetchOpenAIAccountModelsOAuthPopulatesPickerFields(t *testing.T) {
	_, calls := newCodexModelsOAuthCacheServer(t, `{"models":[
		{"slug":"new-oauth-model","display_name":"New OAuth Model"},
		{"slug":"gpt-5.6-sol"},
		{"slug":"blank-display-name","display_name":"   "},
		{"slug":"gpt-6-astra"}
	]}`)
	gateway := &OpenAIGatewayService{}
	svc := &AccountTestService{openaiGatewayService: gateway}
	account := newCodexModelsTestAccount()
	ctx := context.Background()

	before, err := gateway.FetchOpenAIModelsList(ctx, account)
	require.NoError(t, err)
	models, err := svc.FetchOpenAIAccountModels(ctx, account)
	require.NoError(t, err)
	require.Greater(t, len(models), 2)
	// Upstream display name wins; a missing one falls back to the local catalog name,
	// then to the raw slug.
	for i, expected := range []struct{ id, displayName string }{
		{id: "new-oauth-model", displayName: "New OAuth Model"},
		{id: "gpt-5.6-sol", displayName: "GPT-5.6 Sol"},
		{id: "blank-display-name", displayName: "blank-display-name"},
		{id: "gpt-6-astra", displayName: "GPT-6 Astra"},
	} {
		require.Equal(t, expected.id, models[i].ID)
		require.Equal(t, expected.displayName, models[i].DisplayName)
		require.Equal(t, "model", models[i].Type)
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	require.Contains(t, ids, "gpt-image-2.5-flare")
	require.Contains(t, ids, "gpt-image-2.5-sunburst")

	after, err := gateway.FetchOpenAIModelsList(ctx, account)
	require.NoError(t, err)
	require.Equal(t, before.Body, after.Body, "picker fields must not change the shared catalog")
	require.Contains(t, string(after.Body), `"display_name":"New OAuth Model"`, "the shared catalog keeps the upstream display name")
	require.EqualValues(t, 1, calls.Load(), "picker must reuse the shared discovery cache")
}

func TestFetchOpenAIAccountModelsAPIKeyPopulatesPickerFields(t *testing.T) {
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[
			{"id":"new-api-model","owned_by":"provider","created":123},
			{"id":"blank-label","display_name":"  ","type":""},
			{"id":"named-model","display_name":"Provider Model","type":"model"}
		]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), newCodexModelsAPIKeyTestAccount("https://models.example/v1"))
	require.NoError(t, err)
	require.Len(t, models, 3)
	for i, name := range []string{"new-api-model", "blank-label", "Provider Model"} {
		require.Equal(t, name, models[i].DisplayName)
		require.Equal(t, "model", models[i].Type)
	}
	require.Equal(t, "provider", models[0].OwnedBy)
	require.EqualValues(t, 123, models[0].Created)
	require.Equal(t, "named-model", models[2].ID)
}

func TestFetchOpenAIAccountModelsAPIKeyAppliesAccountModelMapping(t *testing.T) {
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[
			{"id":"upstream-target","owned_by":"provider","created":123},
			{"id":"direct-model","owned_by":"provider"},
			{"id":"unconfigured-model","owned_by":"provider"}
		]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	account.Credentials["model_mapping"] = map[string]any{
		"configured-alias": "upstream-target",
		"direct-model":     "direct-model",
	}

	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, models, 2)
	ids := []string{models[0].ID, models[1].ID}
	require.ElementsMatch(t, []string{"configured-alias", "direct-model"}, ids)
	require.NotContains(t, ids, "unconfigured-model")
	alias := models[0]
	if alias.ID != "configured-alias" {
		alias = models[1]
	}
	require.Equal(t, "configured-alias", alias.ID)
	require.Equal(t, "configured-alias", alias.DisplayName)
	require.Equal(t, "provider", alias.OwnedBy)
	require.EqualValues(t, 123, alias.Created)
}

func TestFetchOpenAIAccountModelsAPIKeyKeepsConfiguredRelayRoutes(t *testing.T) {
	for _, catalog := range []string{
		`{"data":[{"id":"openai/gpt-6.1-sol"},{"id":"deepseek/deepseek-v4.1-flash"}]}`,
		`{"data":[]}`,
	} {
		t.Run(catalog, func(t *testing.T) {
			var calls atomic.Int32
			gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				calls.Add(1)
				return ordinaryModelsUpstreamResponse(catalog), nil
			}})
			svc := &AccountTestService{openaiGatewayService: gateway}
			account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
			account.Credentials["model_mapping"] = map[string]any{
				"gpt-6.1-sol":         "gpt-6.1-sol",
				"deepseek-v4.1-flash": "cline-pass/deepseek-v4.1-flash",
				"wild-*":              "unlisted-target",
				"":                    "unlisted-target",
				"blank-target":        "  ",
				"wild-target":         "provider/*",
				" padded-alias ":      "unlisted-target",
			}
			ctx := context.Background()
			before, err := gateway.FetchOpenAIModelsList(ctx, account)
			require.NoError(t, err)
			original := append([]byte(nil), before.Body...)

			models, err := svc.FetchOpenAIAccountModels(ctx, account)
			require.NoError(t, err)
			require.Equal(t, []string{"deepseek-v4.1-flash", "gpt-6.1-sol"}, pickerModelIDs(models))
			for _, model := range models {
				require.Equal(t, "model", model.Object)
				require.Equal(t, "model", model.Type)
				require.NotEmpty(t, model.DisplayName)
			}
			require.Equal(t, "cline-pass/deepseek-v4.1-flash", account.GetMappedModel("deepseek-v4.1-flash"), "discovery must not rewrite the subscription route")
			require.Equal(t, "gpt-6.1-sol", account.GetMappedModel("gpt-6.1-sol"))

			after, err := gateway.FetchOpenAIModelsList(ctx, account)
			require.NoError(t, err)
			require.Equal(t, original, after.Body, "test choices must not enter the shared cache")
			publicBody, err := projectAccountModelsBody(after.Body, account, nil, false)
			require.NoError(t, err)
			var publicCatalog struct {
				Data []openai.Model `json:"data"`
			}
			require.NoError(t, json.Unmarshal(publicBody, &publicCatalog))
			require.Empty(t, publicCatalog.Data, "public discovery remains constrained by upstream availability")
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestFetchOpenAIAccountModelsCombinesDiscoveredAndUnlistedMappings(t *testing.T) {
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[
			{"id":"listed-target","owned_by":"provider","created":123},
			{"id":"wild-known"},{"id":"unconfigured"}
		]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	account.Credentials["model_mapping"] = map[string]any{
		"listed-alias": "listed-target",
		"custom-alias": "subscription/unlisted-target",
		"wild-*":       "listed-target",
	}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"listed-alias", "custom-alias", "wild-known"}, pickerModelIDs(models))
	for _, model := range models {
		if model.ID == "listed-alias" || model.ID == "wild-known" {
			require.Equal(t, "provider", model.OwnedBy)
			require.EqualValues(t, 123, model.Created)
		}
	}
}

func TestFetchOpenAIAccountModelsOAuthKeepsUnlistedConcreteMappings(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"unconfigured-text"}]}`)
	svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
	account := newCodexModelsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"custom-text": "unlisted-text-target"}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"custom-text"}, pickerModelIDs(models))
}

func TestFetchOpenAIAccountModelsPreservesEmptyCatalog(t *testing.T) {
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), newCodexModelsAPIKeyTestAccount("https://models.example/v1"))
	require.NoError(t, err)
	require.Empty(t, models, "an empty upstream catalog must not become a static model list")
}

func TestFetchOpenAIAccountModelsOAuthLabelsLocalImageModelsLikeUpstream(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-5.6-sol"}]}`)
	svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
	account := newCodexModelsTestAccount()
	// This case is about label sources, so it must configure the upstream slug it
	// asserts on: the picker is scoped to the account mapping, and an unconfigured
	// slug is intentionally absent (see the exclusion test below).
	account.Credentials["model_mapping"] = map[string]any{
		"gpt-5.6-sol":         "gpt-5.6-sol",
		"gpt-image-2.5-flare": "gpt-image-2.5-flare",
	}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	byID := make(map[string]string, len(models))
	for _, model := range models {
		byID[model.ID] = model.DisplayName
	}
	require.Equal(t, "GPT-5.6 Sol", byID["gpt-5.6-sol"], "upstream slug must not be the only label source")
	require.Equal(t, "GPT Image 2.5 Flare", byID["gpt-image-2.5-flare"], "locally added models use the same naming rule")
}

func TestFetchOpenAIAccountModelsOAuthRespectsImageAllowlist(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-6-astra"}]}`)
	svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
	account := newCodexModelsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"gpt-image-2.5-flare": "gpt-image-2.5-flare"}
	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	ids := []string{}
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	require.Contains(t, ids, "gpt-image-2.5-flare")
	require.NotContains(t, ids, "gpt-image-2.5-sunburst")
}

func pickerModelIDs(models []openai.Model) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

// The picker is scoped to the account mapping: an upstream slug the account does
// not configure must not become a testable choice.
func TestFetchOpenAIAccountModelsOAuthExcludesUnconfiguredTextModels(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-6-astra"}]}`)
	svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
	account := newCodexModelsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"gpt-5.6-sol": "gpt-5.6-sol"}

	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.6-sol"}, pickerModelIDs(models))
}

// An alias may point at a local image model that Codex discovery never lists, so
// the picker has to resolve the target before deciding to offer the public name.
func TestFetchOpenAIAccountModelsOAuthLocalImageAlias(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-6-astra"}]}`)
	svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
	account := newCodexModelsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"paint": "gpt-image-2.5-flare"}

	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"paint"}, pickerModelIDs(models))
	require.Equal(t, "paint", models[0].DisplayName)
}

// Image-shaped aliases remain selectable even when their text target is absent.
// Test dispatch resolves the target before deciding between text and image APIs.
func TestFetchOpenAIAccountModelsOAuthImageNamesMappedToText(t *testing.T) {
	for _, publicID := range []string{"gpt-image-2.5-lookalike", "gpt-image-2.5-flare"} {
		for _, target := range []string{"text-target-missing", "gpt-6-astra"} {
			t.Run(publicID+"/"+target, func(t *testing.T) {
				newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-6-astra","display_name":"Upstream Text Model"}]}`)
				svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
				account := newCodexModelsTestAccount()
				account.Credentials["model_mapping"] = map[string]any{publicID: target}

				models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
				require.NoError(t, err)
				require.Equal(t, []string{publicID}, pickerModelIDs(models))
				if target == "text-target-missing" {
					require.Equal(t, openaiCodexDisplayName(publicID), models[0].DisplayName)
				} else {
					require.Equal(t, publicID, models[0].DisplayName, "a discovered text target keeps the projected alias label")
				}
			})
		}
	}
}

func TestFetchOpenAIAccountModelsOAuthPassthroughIgnoresMappingTargets(t *testing.T) {
	for _, flag := range []string{"openai_passthrough", "openai_oauth_passthrough"} {
		t.Run(flag, func(t *testing.T) {
			newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-6-astra"}]}`)
			svc := &AccountTestService{openaiGatewayService: &OpenAIGatewayService{}}
			account := newCodexModelsTestAccount()
			account.Extra = map[string]any{flag: true}
			account.Credentials["model_mapping"] = map[string]any{
				"gpt-image-custom":    "text-target-missing",
				"gpt-image-2.5-flare": "text-target-missing",
				"paint":               "gpt-image-2.5-flare",
			}

			models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
			require.NoError(t, err)
			ids := pickerModelIDs(models)
			require.Contains(t, ids, "gpt-6-astra", "passthrough preserves the upstream catalog")
			require.Contains(t, ids, "gpt-image-2.5-flare")
			require.Contains(t, ids, "gpt-image-2.5-sunburst")
			require.Contains(t, ids, "gpt-image-custom", "preserve existing native image choices in passthrough")
			require.NotContains(t, ids, "paint", "passthrough must not revive a stale mapping alias")
		})
	}
}

func TestFetchOpenAIAccountModelsAPIKeyPassthroughIgnoresMapping(t *testing.T) {
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return ordinaryModelsUpstreamResponse(`{"data":[{"id":"target"},{"id":"other"}]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	account.Extra = map[string]any{"openai_passthrough": true}
	account.Credentials["model_mapping"] = map[string]any{"paint": "gpt-image-2.5-flare"}

	models, err := svc.FetchOpenAIAccountModels(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"target", "other"}, pickerModelIDs(models))
}

func TestFetchOpenAIAccountModelsMappingChangesReuseRawCache(t *testing.T) {
	var calls atomic.Int32
	gateway := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		return ordinaryModelsUpstreamResponse(`{"data":[
			{"id":"target","owned_by":"provider","created":123},
			{"id":"gpt-a"},
			{"id":"gpt-special"},
			{"id":"other","owned_by":"other-provider","created":456}
		]}`), nil
	}})
	svc := &AccountTestService{openaiGatewayService: gateway}
	account := newCodexModelsAPIKeyTestAccount("https://models.example/v1")
	account.Credentials["model_mapping"] = map[string]any{"gpt-*": "target", "gpt-special": "other"}
	ctx := context.Background()

	before, err := gateway.FetchOpenAIModelsList(ctx, account)
	require.NoError(t, err)
	original := append([]byte(nil), before.Body...)
	models, err := svc.FetchOpenAIAccountModels(ctx, account)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"gpt-a", "gpt-special"}, pickerModelIDs(models))
	for _, model := range models {
		switch model.ID {
		case "gpt-a":
			require.EqualValues(t, 123, model.Created)
			require.Equal(t, "provider", model.OwnedBy)
		case "gpt-special":
			require.EqualValues(t, 456, model.Created, "exact mapping takes priority over the wildcard")
			require.Equal(t, "other-provider", model.OwnedBy)
		}
	}

	account.Credentials["model_mapping"] = map[string]any{"second": "other"}
	models, err = svc.FetchOpenAIAccountModels(ctx, account)
	require.NoError(t, err)
	require.Equal(t, []string{"second"}, pickerModelIDs(models), "mapping changes apply without refreshing discovery")
	account.Credentials["model_mapping"] = map[string]any{"third": "subscription/unlisted-target"}
	models, err = svc.FetchOpenAIAccountModels(ctx, account)
	require.NoError(t, err)
	require.Equal(t, []string{"third"}, pickerModelIDs(models), "unlisted mappings also update without refreshing discovery")
	after, err := gateway.FetchOpenAIModelsList(ctx, account)
	require.NoError(t, err)
	require.Equal(t, original, after.Body, "projection must not mutate the shared catalog")
	require.EqualValues(t, 1, calls.Load(), "both mappings must reuse the same discovery cache")
}
