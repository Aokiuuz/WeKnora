package core

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/sirupsen/logrus"
)

func TestGetTenantAccessTokenDoesNotLogTokenFragments(t *testing.T) {
	const token = "prefix88-super-secret-token-tail"

	mux := http.NewServeMux()
	mux.HandleFunc("/open-apis/auth/v3/tenant_access_token/internal", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, TokenResponse{
			ApiResponse:       ApiResponse{Code: 0},
			TenantAccessToken: token,
			Expire:            7200,
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	var logs bytes.Buffer
	testLogger := logrus.New()
	testLogger.SetOutput(&logs)
	ctx := context.WithValue(
		context.Background(),
		types.LoggerContextKey,
		logrus.NewEntry(testLogger),
	)

	client := NewClient(&Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL})
	actual, err := client.GetTenantAccessToken(ctx)
	if err != nil {
		t.Fatalf("GetTenantAccessToken() error = %v", err)
	}
	if actual != token {
		t.Fatal("GetTenantAccessToken() returned an unexpected token")
	}

	output := logs.String()
	if !strings.Contains(output, "refreshed tenant_access_token") {
		t.Fatalf("expected token refresh diagnostic, got %q", output)
	}
	for _, fragment := range []string{token, token[:8], token[len(token)-4:]} {
		if strings.Contains(output, fragment) {
			t.Fatalf("tenant access token fragment leaked into logs: %q", output)
		}
	}
}
