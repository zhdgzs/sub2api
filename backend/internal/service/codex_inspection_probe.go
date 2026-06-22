package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const codexInspectionMaxProbeBodyBytes = 8 << 20

type OpenAICodexUsageProbe struct {
	tokenProvider  *OpenAITokenProvider
	settingService *SettingService
}

func NewOpenAICodexUsageProbe(tokenProvider *OpenAITokenProvider, settingService *SettingService) *OpenAICodexUsageProbe {
	return &OpenAICodexUsageProbe{tokenProvider: tokenProvider, settingService: settingService}
}

func (p *OpenAICodexUsageProbe) Probe(ctx context.Context, account *Account, cfg CodexInspectionProbeConfig) CodexInspectionProbeOutcome {
	start := time.Now()
	out := CodexInspectionProbeOutcome{
		ProbeStatus:    CodexInspectionProbeStatusFailed,
		LongWindowType: CodexInspectionLongWindowNone,
		RawRateLimit:   map[string]any{},
	}
	if account == nil {
		out.Error = "account is nil"
		return out
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		out.ProbeStatus = CodexInspectionProbeStatusSkipped
		out.Error = "only openai oauth accounts are supported"
		return out
	}
	if p == nil || p.tokenProvider == nil {
		out.Error = "openai token provider is unavailable"
		return out
	}

	timeout := time.Duration(normalizeProbeConfig(cfg).TimeoutMS) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	token, err := p.tokenProvider.GetAccessToken(reqCtx, account)
	if err != nil {
		out.Error = fmt.Sprintf("get access token: %v", err)
		return out
	}

	client, err := codexInspectionHTTPClient(account, timeout)
	if err != nil {
		out.Error = fmt.Sprintf("build http client: %v", err)
		return out
	}

	attempts := normalizeProbeConfig(cfg).Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-reqCtx.Done():
				out.Error = reqCtx.Err().Error()
				return out
			case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
			}
		}
		attemptOut, retry := p.doProbe(reqCtx, client, account, token, cfg, start)
		out = attemptOut
		if !retry {
			return out
		}
	}
	return out
}

func (p *OpenAICodexUsageProbe) doProbe(ctx context.Context, client *http.Client, account *Account, token string, cfg CodexInspectionProbeConfig, start time.Time) (CodexInspectionProbeOutcome, bool) {
	out := CodexInspectionProbeOutcome{
		ProbeStatus:    CodexInspectionProbeStatusFailed,
		LongWindowType: CodexInspectionLongWindowNone,
		RawRateLimit:   map[string]any{},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexInspectionUsageURL, nil)
	if err != nil {
		out.Error = fmt.Sprintf("build request: %v", err)
		return out, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.resolveUserAgent(ctx, cfg))
	if chatgptAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); chatgptAccountID != "" {
		req.Header.Set("Chatgpt-Account-Id", chatgptAccountID)
	}

	resp, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	out.LatencyMS = &latency
	if err != nil {
		out.Error = fmt.Sprintf("usage request: %v", err)
		return out, true
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	out.StatusCode = &statusCode
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, codexInspectionMaxProbeBodyBytes))
	if readErr != nil {
		out.Error = fmt.Sprintf("read response body: %v", readErr)
		return out, statusCode >= 500
	}

	bodyText := string(body)
	out.BodyText = bodyText
	out.BodyExcerpt = truncateCodexInspectionText(logredact.RedactText(bodyText, "authorization", "token"), codexInspectionMaxStoredBodyText)
	out.RawRateLimit, out.Windows = parseCodexInspectionRateLimit(body)
	fillCodexInspectionProbeWindowSummary(&out)
	out.ProbeStatus = CodexInspectionProbeStatusSuccess
	return out, false
}

func (p *OpenAICodexUsageProbe) resolveUserAgent(ctx context.Context, cfg CodexInspectionProbeConfig) string {
	if ua := strings.TrimSpace(cfg.UserAgent); ua != "" {
		return ua
	}
	if p != nil && p.settingService != nil {
		if ua := strings.TrimSpace(p.settingService.GetOpenAICodexUserAgent(ctx)); ua != "" {
			return ua
		}
	}
	return DefaultOpenAICodexUserAgent
}

func codexInspectionHTTPClient(account *Account, timeout time.Duration) (*http.Client, error) {
	proxyURL := ""
	if account != nil && account.Proxy != nil && account.Proxy.IsActive() {
		proxyURL = account.Proxy.URL()
	}
	return httppool.GetClient(httppool.Options{
		ProxyURL:              proxyURL,
		Timeout:               timeout,
		ResponseHeaderTimeout: timeout,
	})
}

func parseCodexInspectionRateLimit(raw []byte) (map[string]any, []CodexInspectionRateLimitWindow) {
	var decoded any
	if len(raw) == 0 || json.Unmarshal(raw, &decoded) != nil {
		return map[string]any{}, nil
	}
	root, _ := decoded.(map[string]any)
	rateLimit := findCodexInspectionRateLimitObject(decoded)
	if rateLimit == nil {
		rateLimit = root
	}
	if rateLimit == nil {
		return map[string]any{}, nil
	}
	windows := make([]CodexInspectionRateLimitWindow, 0, 4)
	collectCodexInspectionWindows(rateLimit, &windows, 0)
	return rateLimit, dedupeCodexInspectionWindows(windows)
}

func findCodexInspectionRateLimitObject(value any) map[string]any {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"rate_limit", "rate_limits", "rateLimit", "rateLimits", "limits", "usage"} {
			if child, ok := v[key]; ok {
				if m, ok := child.(map[string]any); ok {
					return m
				}
				if arr, ok := child.([]any); ok {
					return map[string]any{key: arr}
				}
			}
		}
		for _, child := range v {
			if found := findCodexInspectionRateLimitObject(child); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range v {
			if found := findCodexInspectionRateLimitObject(child); found != nil {
				return found
			}
		}
	}
	return nil
}

func collectCodexInspectionWindows(value any, out *[]CodexInspectionRateLimitWindow, depth int) {
	if depth > 16 {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		if w, ok := codexInspectionWindowFromMap(v); ok {
			*out = append(*out, w)
		}
		for _, child := range v {
			collectCodexInspectionWindows(child, out, depth+1)
		}
	case []any:
		for _, child := range v {
			collectCodexInspectionWindows(child, out, depth+1)
		}
	}
}

func codexInspectionWindowFromMap(m map[string]any) (CodexInspectionRateLimitWindow, bool) {
	seconds := firstIntFromMap(m, "window_seconds", "windowSeconds", "period_seconds", "periodSeconds", "duration_seconds", "durationSeconds")
	minutes := firstIntFromMap(m, "window_minutes", "windowMinutes", "period_minutes", "periodMinutes", "duration_minutes", "durationMinutes")
	if seconds <= 0 && minutes <= 0 {
		if raw := strings.ToLower(firstStringFromMap(m, "window", "period", "duration")); raw != "" {
			seconds, minutes = parseCodexInspectionWindowText(raw)
		}
	}
	if seconds <= 0 && minutes > 0 {
		seconds = minutes * 60
	}
	if minutes <= 0 && seconds > 0 {
		minutes = seconds / 60
	}

	usedPercent := firstFloatPtrFromMap(m, "used_percent", "usedPercent", "usage_percent", "usagePercent", "percent_used", "percentUsed")
	if usedPercent == nil {
		usedPercent = computeUsedPercentFromLimit(m)
	}
	allowed := firstBoolPtrFromMap(m, "allowed", "is_allowed", "isAllowed")
	limitReached := firstBoolFromMap(m, "limit_reached", "limitReached", "exhausted", "quota_exhausted", "quotaExhausted")
	if allowed != nil && !*allowed {
		limitReached = true
	}
	if seconds <= 0 && minutes <= 0 && usedPercent == nil && !limitReached {
		return CodexInspectionRateLimitWindow{}, false
	}
	return CodexInspectionRateLimitWindow{
		WindowSeconds: seconds,
		WindowMinutes: minutes,
		UsedPercent:   usedPercent,
		Allowed:       allowed,
		LimitReached:  limitReached,
		Raw:           m,
	}, true
}

func parseCodexInspectionWindowText(raw string) (seconds int, minutes int) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "5h", "5_hour", "5-hour", "five_hour", "five-hour":
		return codexInspectionFiveHourSeconds, 300
	case "weekly", "week", "7d", "7_day", "7-day":
		return codexInspectionWeekSeconds, 10_080
	case "monthly", "month", "30d", "30_day", "30-day":
		return codexInspectionMonthSeconds, 43_200
	default:
		return 0, 0
	}
}

func computeUsedPercentFromLimit(m map[string]any) *float64 {
	used, okUsed := firstFloatFromMap(m, "used", "usage", "consumed")
	limit, okLimit := firstFloatFromMap(m, "limit", "max", "quota")
	if okUsed && okLimit && limit > 0 {
		v := used / limit * 100
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return &v
		}
	}
	remaining, okRemaining := firstFloatFromMap(m, "remaining")
	if okRemaining && okLimit && limit > 0 {
		v := (limit - remaining) / limit * 100
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return &v
		}
	}
	return nil
}

func dedupeCodexInspectionWindows(in []CodexInspectionRateLimitWindow) []CodexInspectionRateLimitWindow {
	out := make([]CodexInspectionRateLimitWindow, 0, len(in))
	seen := map[string]struct{}{}
	for _, w := range in {
		key := fmt.Sprintf("%d:%d:%v:%v", w.WindowSeconds, w.WindowMinutes, w.UsedPercent, w.LimitReached)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, w)
	}
	return out
}

func truncateCodexInspectionText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func firstStringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return ""
}

func firstIntFromMap(m map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if i, ok := anyToInt(v); ok {
				return i
			}
		}
	}
	return 0
}

func firstFloatPtrFromMap(m map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if f, ok := anyToFloat64(v); ok {
				return &f
			}
		}
	}
	return nil
}

func firstFloatFromMap(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return anyToFloat64(v)
		}
	}
	return 0, false
}

func firstBoolPtrFromMap(m map[string]any, keys ...string) *bool {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if b, ok := anyToBool(v); ok {
				return &b
			}
		}
	}
	return nil
}

func firstBoolFromMap(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if b, ok := anyToBool(v); ok {
				return b
			}
		}
	}
	return false
}

func anyToInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	case string:
		var i int
		_, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &i)
		return i, err == nil
	default:
		return 0, false
	}
}

func anyToFloat64(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		var f float64
		_, err := fmt.Sscanf(strings.TrimSuffix(strings.TrimSpace(t), "%"), "%f", &f)
		return f, err == nil
	default:
		return 0, false
	}
}

func anyToBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	}
	return false, false
}
