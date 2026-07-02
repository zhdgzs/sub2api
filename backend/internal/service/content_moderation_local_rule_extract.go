package service

import (
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const (
	defaultContentModerationLocalRulesHeadScanLength = 64 * 1024
	defaultContentModerationLocalRulesTailScanLength = 16 * 1024
)

func ExtractContentModerationFullTextContext(protocol string, endpoint string, body []byte, maxLen int) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	var parts []string
	addResultText := func(result gjson.Result) {
		if result.Exists() {
			collectContentModerationFullTextContext(result, &parts)
		}
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case protocol == ContentModerationProtocolOpenAIChat || endpoint == "chat" || endpoint == "chat_completions" || endpoint == "/v1/chat/completions":
		addResultText(gjson.GetBytes(body, "messages"))
	case protocol == ContentModerationProtocolAnthropicMessages || endpoint == "messages" || endpoint == "anthropic" || endpoint == "/v1/messages":
		addResultText(gjson.GetBytes(body, "system"))
		addResultText(gjson.GetBytes(body, "messages"))
	case protocol == ContentModerationProtocolOpenAIImages || endpoint == "image" || endpoint == "images" || endpoint == "images_generations" || endpoint == "images_edits" || endpoint == "/v1/images/generations" || endpoint == "/v1/images/edits":
		addResultText(gjson.GetBytes(body, "prompt"))
		addResultText(gjson.GetBytes(body, "style"))
	case protocol == ContentModerationProtocolGemini:
		addResultText(gjson.GetBytes(body, "contents"))
	default:
		addResultText(gjson.GetBytes(body, "instructions"))
		addResultText(gjson.GetBytes(body, "input"))
		addResultText(gjson.GetBytes(body, "prompt"))
		addResultText(gjson.GetBytes(body, "messages"))
		addResultText(gjson.GetBytes(body, "contents"))
	}
	return limitContentModerationLocalRuleScanText(strings.Join(parts, "\n"), maxLen)
}

func collectContentModerationFullTextContext(result gjson.Result, parts *[]string) {
	if !result.Exists() || result.Type == gjson.Null {
		return
	}
	switch {
	case result.IsArray():
		for _, item := range result.Array() {
			collectContentModerationFullTextContext(item, parts)
		}
	case result.IsObject():
		if textValue := result.Get("text"); textValue.Type == gjson.String {
			if text := strings.TrimSpace(textValue.String()); text != "" {
				*parts = append(*parts, text)
			}
		}
		if contentValue := result.Get("content"); contentValue.Exists() {
			if contentValue.Type == gjson.String {
				if text := strings.TrimSpace(contentValue.String()); text != "" {
					*parts = append(*parts, text)
				}
			} else {
				collectContentModerationFullTextContext(contentValue, parts)
			}
		}
		result.ForEach(func(key, value gjson.Result) bool {
			if shouldSkipContentModerationFullTextContextKey(key.String()) {
				return true
			}
			collectContentModerationFullTextContext(value, parts)
			return true
		})
	case result.Type == gjson.String:
		if text := strings.TrimSpace(result.String()); text != "" {
			*parts = append(*parts, text)
		}
	}
}

func shouldSkipContentModerationFullTextContextKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "text", "content", "image_url", "url", "file_id", "result", "data", "b64_json", "source", "file", "type", "role",
		"inline_data", "inlinedata", "file_data", "filedata":
		return true
	default:
		return false
	}
}

func limitContentModerationLocalRuleScanText(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = defaultContentModerationLocalRulesMaxTextLength
	}
	if len(text) <= maxLen {
		return text
	}
	head := defaultContentModerationLocalRulesHeadScanLength
	tail := defaultContentModerationLocalRulesTailScanLength
	if maxLen < head+tail {
		head = maxLen * 4 / 5
		tail = maxLen - head
	}
	if head > len(text) {
		head = len(text)
	}
	if tail > len(text)-head {
		tail = len(text) - head
	}
	return safeContentModerationLocalRuleUTF8Prefix(text, head) + "\n" + safeContentModerationLocalRuleUTF8Suffix(text, tail)
}

func safeContentModerationLocalRuleUTF8Prefix(text string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if maxBytes >= len(text) {
		return text
	}
	for maxBytes > 0 && !utf8.ValidString(text[:maxBytes]) {
		maxBytes--
	}
	return text[:maxBytes]
}

func safeContentModerationLocalRuleUTF8Suffix(text string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if maxBytes >= len(text) {
		return text
	}
	start := len(text) - maxBytes
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}
