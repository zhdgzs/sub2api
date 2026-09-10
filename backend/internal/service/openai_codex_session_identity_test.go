package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func requireCodexSessionParity(t *testing.T, headers http.Header, body []byte) {
	t.Helper()
	cm := gjson.GetBytes(body, "client_metadata")
	embedded := codexConvergenceEmbeddedMetadata(cm)
	headerMetadata := gjson.Parse(headers.Get(openAIWSTurnMetadataHeader))
	for _, field := range codexSessionIdentityFields {
		want := headers.Get(field[1])
		require.NotEmpty(t, want)
		require.Equal(t, want, cm.Get(field[0]).String())
		for _, metadata := range []gjson.Result{cm, embedded, headerMetadata} {
			for _, name := range field {
				if value := metadata.Get(name); value.Exists() {
					require.Equal(t, want, value.String(), name)
				}
			}
		}
	}
	require.Empty(t, headers.Get("session_id"))
	require.Empty(t, headers.Get("conversation_id"))
}

func TestCodexSessionIdentityHTTPEntryMatrix(t *testing.T) {
	for _, mode := range []string{"off", "device", "session", "full"} {
		for _, enabled := range []bool{false, true} {
			for _, entry := range []string{"responses", "passthrough", "messages", "chat"} {
				t.Run(fmt.Sprintf("%s/experiment=%v/%s", mode, enabled, entry), func(t *testing.T) {
					account := wireProfileTestAccount(enabled)
					account.Extra[codexFingerprintModeExtraKey] = mode
					account.Extra["openai_passthrough"] = entry == "passthrough"
					body := wireProfileTestBody(t)
					var err error
					body, err = sjson.SetBytes(body, "client_metadata.session-id", "conflicting-alias")
					require.NoError(t, err)
					body, err = sjson.SetBytes(body, "client_metadata.x-codex-turn-metadata", `{"session_id":"conflicting-embedded","session-id":"other-alias","thread_id":"other-thread","sandbox":"keep"}`)
					require.NoError(t, err)
					if entry == "messages" || entry == "chat" {
						body = []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
					}
					c := newConvTestContext(t, body)
					c.Request.Header.Set("session-id", "conflicting-header")
					c.Request.Header.Set("session_id", "conflicting-legacy")
					svc, upstream := wireProfileTestService()
					upstream.resp = openAICompatSSECompletedResponse("resp_session_identity", "gpt-5.4")
					switch entry {
					case "messages":
						_, err = svc.ForwardAsAnthropic(context.Background(), c, account, body, "independent-cache-key", "gpt-5.4")
					case "chat":
						_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, "independent-cache-key", "gpt-5.4")
					default:
						_, err = svc.Forward(context.Background(), c, account, body)
					}
					require.NoError(t, err)
					require.NotNil(t, upstream.lastReq, "request must reach the recording upstream")
					requireCodexSessionParity(t, upstream.lastReq.Header, upstream.lastBody)
					if (mode == "off" || mode == "device") && (entry == "responses" || entry == "passthrough") {
						require.Equal(t, scopeCodexAccountIdentityValue(account, 77, "session", convTestSession), upstream.lastReq.Header.Get("session-id"), "body wins over conflicting headers")
					}
				})
			}
		}
	}
}

func TestCodexSessionIdentityLegacyHeaderAndBodyOnly(t *testing.T) {
	for _, carrier := range []string{"legacy", "embedded", "body"} {
		t.Run(carrier, func(t *testing.T) {
			account := convTestAccount(false)
			c := newConvTestContext(t, nil)
			c.Request.Header = make(http.Header)
			body := map[string]any{}
			switch carrier {
			case "legacy":
				c.Request.Header.Set("session_id", "raw-session")
			case "embedded":
				body["client_metadata"] = map[string]any{openAIWSTurnMetadataHeader: `{"session_id":"raw-session"}`}
			case "body":
				body["client_metadata"] = map[string]any{"session_id": "raw-session"}
			}
			normalizeCodexSessionIdentityMap(c, account, body)
			applyCodexAccountIdentityClientMetadataMap(body, account, 77)
			stageCodexConvergenceBodyIdentityMap(c, account, body)
			headers := make(http.Header)
			applyCodexFingerprintConvergenceHeaders(c, account, headers)
			require.Equal(t, scopeCodexAccountIdentityValue(account, 77, "session", "raw-session"), headers.Get("session-id"))
			require.Empty(t, headers.Get("session_id"))
		})
	}
}

func TestCodexWSSessionIdentityRequiresReconnect(t *testing.T) {
	for _, mode := range []string{"off", "device", "session", "full"} {
		for _, enabled := range []bool{false, true} {
			for _, changedField := range []string{"session_id", "thread_id"} {
				for _, event := range []string{"response.create", "session.update"} {
					t.Run(fmt.Sprintf("%s/%v/%s/%s", mode, enabled, changedField, event), func(t *testing.T) {
						account := wireProfileTestAccount(enabled)
						account.Extra[codexFingerprintModeExtraKey] = mode
						c := newConvTestContext(t, nil)
						c.Request.Header.Set("session-id", "conflicting-header")
						first, err := applyCodexIdentityToWSPayload(c, account, convTestBody(t))
						require.NoError(t, err)
						headers, _, err := (&OpenAIGatewayService{}).buildOpenAIWSHeaders(context.Background(), c, account, "offline-token",
							OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", convTestTurnMetadata(), convTestSession, "", "")
						require.NoError(t, err)
						requireCodexSessionParity(t, headers, first)
						// 缺省会话身份继承首帧，窗口递增不应要求重连。
						next, err := applyCodexIdentityToWSPayload(c, account, []byte(`{"type":"response.create","client_metadata":{"x-codex-window-id":"`+convTestThread+`:9"}}`))
						require.NoError(t, err)
						requireCodexSessionParity(t, headers, next)
						require.Contains(t, gjson.GetBytes(next, "client_metadata.x-codex-window-id").String(), ":9")
						request := map[string]any{"client_metadata": map[string]any{changedField: "different-session"}}
						if event == "session.update" {
							request = map[string]any{"session": request}
						}
						request["type"] = event
						raw, err := json.Marshal(request)
						require.NoError(t, err)
						_, err = applyCodexIdentityToWSPayload(c, account, raw)
						require.ErrorContains(t, err, "reconnect")
					})
				}
			}
		}
	}
}

func TestCodexSessionIdentitySkipsAPIKeyAndRejectsStaleAccountStaging(t *testing.T) {
	account := convTestAccount(false)
	c := newConvTestContext(t, nil)
	stageCodexConvergenceBodyIdentityRaw(c, account, []byte(`{"client_metadata":{"session_id":"scoped-old-account"}}`))
	other := *account
	other.ID++
	require.Empty(t, stagedCodexConvergenceBodyIdentity(c, &other))
	account.Type = AccountTypeAPIKey
	raw := []byte(`{"client_metadata":{"session_id":"raw-session"}}`)
	next, changed, err := normalizeCodexSessionIdentityRaw(c, account, raw)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, raw, next)
	headers := http.Header{"Session_id": []string{"raw-session"}}
	applyCodexFingerprintConvergenceHeaders(c, account, headers)
	require.Equal(t, "raw-session", headers.Get("session_id"))
	require.Empty(t, headers.Get("session-id"))
}
