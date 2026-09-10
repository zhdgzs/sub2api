package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var codexSessionIdentityFields = [][2]string{
	{"session_id", "session-id"},
	{"thread_id", "thread-id"},
}

// 同一请求只选一份原始会话身份：body > 嵌入 metadata > 请求头 > 头部 metadata。
// 必须先统一再派生，避免对已隔离的值重复哈希；缓存键保持独立语义。
func normalizeCodexSessionIdentityMap(c *gin.Context, account *Account, body map[string]any) bool {
	if body == nil || codexAccountIdentityNamespace(account) == "" {
		return false
	}
	cm, _ := body["client_metadata"].(map[string]any)
	if cm == nil {
		cm = map[string]any{}
	}
	var embedded map[string]any
	if raw, ok := cm[openAIWSTurnMetadataHeader].(string); ok {
		_ = json.Unmarshal([]byte(raw), &embedded)
	}
	var inbound http.Header
	if c != nil && c.Request != nil {
		inbound = c.Request.Header
	}
	headerMetadata := gjson.Parse(inbound.Get(openAIWSTurnMetadataHeader))
	var boundSession map[string]string
	if c != nil {
		value, _ := c.Get(codexWSSessionIdentityContextKey)
		boundSession, _ = value.(map[string]string)
	}
	changed, embeddedChanged := false, false
	for _, field := range codexSessionIdentityFields {
		value := codexSessionIdentityValue(cm, field)
		if value == "" {
			value = codexSessionIdentityValue(embedded, field)
		}
		if value == "" {
			value = boundSession[field[0]]
		}
		if value == "" {
			value = strings.TrimSpace(inbound.Get(field[1]))
		}
		if value == "" {
			value = strings.TrimSpace(inbound.Get(field[0]))
		}
		if value == "" {
			value = codexConvergenceMetadataString(headerMetadata, field[0])
		}
		if value != "" {
			changed = setCodexSessionIdentityValue(cm, field, value) || changed
			if embedded != nil {
				embeddedChanged = setCodexSessionIdentityValue(embedded, field, value) || embeddedChanged
			}
		}
	}
	if embeddedChanged {
		raw, err := json.Marshal(embedded)
		if err == nil {
			cm[openAIWSTurnMetadataHeader] = string(raw)
			changed = true
		}
	}
	if changed {
		body["client_metadata"] = cm
	}
	return changed
}

func codexSessionIdentityValue(metadata map[string]any, field [2]string) string {
	for _, name := range field {
		if value, ok := metadata[name].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func setCodexSessionIdentityValue(metadata map[string]any, field [2]string, value string) bool {
	changed := metadata[field[0]] != value
	metadata[field[0]] = value
	if old, exists := metadata[field[1]]; exists && old != value {
		metadata[field[1]] = value
		changed = true
	}
	return changed
}

// 只解析小型 metadata 对象，避免反序列化完整的图片或长上下文请求。
func normalizeCodexSessionIdentityRaw(c *gin.Context, account *Account, body []byte) ([]byte, bool, error) {
	if codexAccountIdentityNamespace(account) == "" || !gjson.ParseBytes(body).IsObject() {
		return body, false, nil
	}
	request := map[string]any{}
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(cm.Raw), &metadata); err != nil {
			return body, false, err
		}
		request["client_metadata"] = metadata
	}
	if !normalizeCodexSessionIdentityMap(c, account, request) {
		return body, false, nil
	}
	next, err := sjson.SetBytes(body, "client_metadata", request["client_metadata"])
	return next, err == nil, err
}

const codexWSSessionIdentityContextKey = "codex_ws_session_identity"

// 握手头在连接存续期间不可修改。缺省身份继承首帧，显式换会话必须重连；轮次和窗口可变化。
func bindCodexWSSessionIdentity(c *gin.Context, account *Account, payload []byte) error {
	if c == nil || codexAccountIdentityNamespace(account) == "" {
		return nil
	}
	value, bound := c.Get(codexWSSessionIdentityContextKey)
	previous, _ := value.(map[string]string)
	metadata := gjson.GetBytes(payload, "client_metadata")
	current := map[string]string{}
	for _, field := range codexSessionIdentityFields {
		identity := codexConvergenceMetadataString(metadata, field[0])
		if bound && identity != previous[field[0]] {
			return fmt.Errorf("websocket %s changed; reconnect to start a different session", field[0])
		}
		current[field[0]] = identity
	}
	if !bound {
		c.Set(codexWSSessionIdentityContextKey, current)
	}
	return nil
}

func syncCodexSessionIdentityHeaderMetadata(headers http.Header) {
	var metadata map[string]any
	if err := json.Unmarshal([]byte(headers.Get(openAIWSTurnMetadataHeader)), &metadata); err != nil || metadata == nil {
		return
	}
	changed := false
	for _, field := range codexSessionIdentityFields {
		if value := headers.Get(field[1]); value != "" {
			changed = setCodexSessionIdentityValue(metadata, field, value) || changed
		}
	}
	if changed {
		if raw, err := json.Marshal(metadata); err == nil {
			headers.Set(openAIWSTurnMetadataHeader, string(raw))
		}
	}
}
