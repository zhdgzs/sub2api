package admin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type applyOAuthStatefulAdminService struct {
	*stubAdminService
	account service.Account
}

func newApplyOAuthStatefulAdminService(account service.Account) *applyOAuthStatefulAdminService {
	return &applyOAuthStatefulAdminService{
		stubAdminService: newStubAdminService(),
		account:          account,
	}
}

func (s *applyOAuthStatefulAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if s.account.ID != id {
		return nil, service.ErrAccountNotFound
	}
	return cloneServiceAccountForApplyOAuthTest(s.account), nil
}

func (s *applyOAuthStatefulAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if s.account.ID != id {
		return nil, service.ErrAccountNotFound
	}
	if input.Type != "" {
		s.account.Type = input.Type
	}
	if len(input.Credentials) > 0 {
		s.account.Credentials = service.MergePreservingSensitiveCreds(s.account.Credentials, input.Credentials)
	}
	return cloneServiceAccountForApplyOAuthTest(s.account), nil
}

func (s *applyOAuthStatefulAdminService) UpdateAccountExtra(_ context.Context, id int64, updates map[string]any) error {
	if s.account.ID != id {
		return service.ErrAccountNotFound
	}
	if s.account.Extra == nil {
		s.account.Extra = make(map[string]any, len(updates))
	}
	for key, value := range updates {
		s.account.Extra[key] = value
	}
	return nil
}

func (s *applyOAuthStatefulAdminService) ClearAccountError(_ context.Context, id int64) (*service.Account, error) {
	if s.account.ID != id {
		return nil, service.ErrAccountNotFound
	}
	s.account.Status = service.StatusActive
	s.account.ErrorMessage = ""
	return cloneServiceAccountForApplyOAuthTest(s.account), nil
}

func cloneServiceAccountForApplyOAuthTest(account service.Account) *service.Account {
	cloned := account
	if account.Credentials != nil {
		cloned.Credentials = make(map[string]any, len(account.Credentials))
		for key, value := range account.Credentials {
			cloned.Credentials[key] = value
		}
	}
	if account.Extra != nil {
		cloned.Extra = make(map[string]any, len(account.Extra))
		for key, value := range account.Extra {
			cloned.Extra[key] = value
		}
	}
	return &cloned
}

type applyOAuthOpenAIClientStub struct {
	idToken string
}

func (s *applyOAuthOpenAIClientStub) ExchangeCode(_ context.Context, _, _, _, _, _ string) (*openai.TokenResponse, error) {
	return nil, errors.New("unexpected exchange code")
}

func (s *applyOAuthOpenAIClientStub) RefreshToken(_ context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(context.Background(), refreshToken, proxyURL, "")
}

func (s *applyOAuthOpenAIClientStub) RefreshTokenWithClientID(_ context.Context, _, _ string, _ string) (*openai.TokenResponse, error) {
	return &openai.TokenResponse{
		AccessToken:  "refreshed-access-token",
		RefreshToken: "refreshed-refresh-token",
		IDToken:      s.idToken,
		ExpiresIn:    3600,
	}, nil
}

func TestAccountHandlerApplyOAuthCredentialsRefreshesOpenAIPlanType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := newApplyOAuthStatefulAdminService(service.Account{
		ID:       42,
		Name:     "openai-account",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusError,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "old-refresh-token",
			"plan_type":     "free",
		},
		Extra: map[string]any{"privacy_mode": "training_off"},
	})
	openaiSvc := service.NewOpenAIOAuthService(nil, &applyOAuthOpenAIClientStub{
		idToken: buildOpenAIIDTokenForApplyOAuthTest(t, "plus"),
	})

	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, openaiSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/apply-oauth-credentials", handler.ApplyOAuthCredentials)

	payload := map[string]any{
		"type": "oauth",
		"credentials": map[string]any{
			"access_token":  "reauthorized-access-token",
			"refresh_token": "reauthorized-refresh-token",
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/apply-oauth-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "plus", adminSvc.account.Credentials["plan_type"])
	require.Equal(t, "refreshed-access-token", adminSvc.account.Credentials["access_token"])

	var responseBody struct {
		Data struct {
			Credentials       map[string]any  `json:"credentials"`
			CredentialsStatus map[string]bool `json:"credentials_status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
	require.Equal(t, "plus", responseBody.Data.Credentials["plan_type"])
	require.NotContains(t, responseBody.Data.Credentials, "access_token")
	require.True(t, responseBody.Data.CredentialsStatus["has_access_token"])
	require.True(t, responseBody.Data.CredentialsStatus["has_refresh_token"])
}

func buildOpenAIIDTokenForApplyOAuthTest(t *testing.T, planType string) string {
	t.Helper()

	header := map[string]any{"alg": "none", "typ": "JWT"}
	payload := map[string]any{
		"email": "openai@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "account-id",
			"chatgpt_user_id":    "user-id",
			"chatgpt_plan_type":  planType,
			"organizations": []map[string]any{
				{"id": "org-id", "is_default": true},
			},
		},
	}

	return encodeJWTPartForApplyOAuthTest(t, header) + "." + encodeJWTPartForApplyOAuthTest(t, payload) + ".signature"
}

func encodeJWTPartForApplyOAuthTest(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(raw)
}
