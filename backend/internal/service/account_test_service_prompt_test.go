//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountTestPayloadsUseCustomPrompt(t *testing.T) {
	t.Parallel()

	const prompt = "你是什么模型"

	claudePayload, err := createTestPayload("claude-sonnet-4-5", prompt)
	require.NoError(t, err)
	claudeBody, err := json.Marshal(claudePayload)
	require.NoError(t, err)
	require.Equal(t, prompt, gjson.GetBytes(claudeBody, "messages.0.content.0.text").String())

	bedrockBody, err := json.Marshal(createBedrockTestPayload(prompt))
	require.NoError(t, err)
	require.Equal(t, prompt, gjson.GetBytes(bedrockBody, "messages.0.content.0.text").String())

	openAIBody, err := json.Marshal(createOpenAITestPayload("gpt-5.4", false, prompt))
	require.NoError(t, err)
	require.Equal(t, prompt, gjson.GetBytes(openAIBody, "input.0.content.0.text").String())

	antigravitySvc := &AntigravityGatewayService{}
	antigravityGeminiBody, err := antigravitySvc.buildGeminiTestRequest("project-1", "gemini-2.5-flash", prompt)
	require.NoError(t, err)
	require.Equal(t, prompt, gjson.GetBytes(antigravityGeminiBody, "request.contents.0.parts.0.text").String())

	antigravityClaudeBody, err := antigravitySvc.buildClaudeTestRequest("project-1", "claude-sonnet-4-5", prompt)
	require.NoError(t, err)
	require.Contains(t, string(antigravityClaudeBody), prompt)
}

func TestAccountTestPayloadsFallbackToDefaultPrompt(t *testing.T) {
	t.Parallel()

	claudePayload, err := createTestPayload("claude-sonnet-4-5", "  ")
	require.NoError(t, err)
	claudeBody, err := json.Marshal(claudePayload)
	require.NoError(t, err)
	require.Equal(t, defaultTextTestPrompt, gjson.GetBytes(claudeBody, "messages.0.content.0.text").String())

	bedrockBody, err := json.Marshal(createBedrockTestPayload(""))
	require.NoError(t, err)
	require.Equal(t, defaultTextTestPrompt, gjson.GetBytes(bedrockBody, "messages.0.content.0.text").String())

	openAIBody, err := json.Marshal(createOpenAITestPayload("gpt-5.4", false, ""))
	require.NoError(t, err)
	require.Equal(t, defaultTextTestPrompt, gjson.GetBytes(openAIBody, "input.0.content.0.text").String())

	antigravitySvc := &AntigravityGatewayService{}
	antigravityGeminiBody, err := antigravitySvc.buildGeminiTestRequest("project-1", "gemini-2.5-flash", "")
	require.NoError(t, err)
	require.Equal(t, defaultTextTestPrompt, gjson.GetBytes(antigravityGeminiBody, "request.contents.0.parts.0.text").String())

	antigravityClaudeBody, err := antigravitySvc.buildClaudeTestRequest("project-1", "claude-sonnet-4-5", "")
	require.NoError(t, err)
	require.Contains(t, string(antigravityClaudeBody), defaultTextTestPrompt)
}
