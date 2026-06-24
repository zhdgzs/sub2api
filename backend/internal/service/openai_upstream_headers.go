package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

type openAIUpstreamUserAgentOptions struct {
	Config                  *config.Config
	SettingService          *SettingService
	DefaultUserAgent        string
	ForceCodexForOAuth      bool
	OverrideBrowserForOAuth bool
}

// applyOpenAIUpstreamUserAgent 统一 OpenAI 上游请求的 User-Agent 优先级。
func applyOpenAIUpstreamUserAgent(ctx context.Context, req *http.Request, account *Account, opts openAIUpstreamUserAgentOptions) {
	if req == nil {
		return
	}

	customUA := ""
	if account != nil {
		customUA = strings.TrimSpace(account.GetOpenAIUserAgent())
	}
	switch {
	case customUA != "":
		req.Header.Set("user-agent", customUA)
	case strings.TrimSpace(opts.DefaultUserAgent) != "":
		req.Header.Set("user-agent", strings.TrimSpace(opts.DefaultUserAgent))
	}

	if opts.Config != nil && opts.Config.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}
	if opts.ForceCodexForOAuth && account != nil && account.Type == AccountTypeOAuth &&
		!openai.IsCodexCLIRequest(req.Header.Get("user-agent")) {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}
	if opts.OverrideBrowserForOAuth {
		overrideOpenAIBrowserUserAgent(ctx, req, account, opts.SettingService)
	}
}

func overrideOpenAIBrowserUserAgent(ctx context.Context, req *http.Request, account *Account, settingService *SettingService) {
	if req == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	currentUA := req.Header.Get("user-agent")
	if !openai.IsBrowserUserAgent(currentUA) {
		return
	}

	codexUA := DefaultOpenAICodexUserAgent
	if settingService != nil {
		if v := strings.TrimSpace(settingService.GetOpenAICodexUserAgent(ctx)); v != "" {
			codexUA = v
		}
	}
	req.Header.Set("user-agent", codexUA)
}

func applyOpenAICodexClientIdentityHeaders(req *http.Request, sessionID string) {
	if req == nil {
		return
	}
	if req.Header.Get("originator") == "" {
		req.Header.Set("originator", "codex_cli_rs")
	}
	if sessionID == "" {
		return
	}
	req.Header.Set("session_id", sessionID)
	req.Header.Set("conversation_id", sessionID)
}

func applyOpenAICodexInternalAPIHeaders(req *http.Request, sessionID string) {
	applyOpenAICodexClientIdentityHeaders(req, sessionID)
	if req == nil {
		return
	}
	if req.Header.Get("OpenAI-Beta") == "" {
		req.Header.Set("OpenAI-Beta", "responses=experimental")
	}
	if req.Header.Get("version") == "" {
		req.Header.Set("version", codexCLIVersion)
	}
}

func openAIProbeSessionID(prefix string, accountID int64) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "probe"
	}
	if accountID <= 0 {
		return prefix
	}
	return prefix + "_" + strconv.FormatInt(accountID, 10)
}
