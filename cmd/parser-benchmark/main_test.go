package main

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestEngineOverridesDoNotSendOtherProvidersCredentials(t *testing.T) {
	c := &types.ParserEngineConfig{MinerUAPIKey: "mineru-secret", PaddleOCRVLCloudToken: "paddle-secret", MinerUEndpoint: "http://mineru:8000", PaddleOCRVLEndpoint: "http://paddle:8080"}
	for _, engine := range []string{"builtin", "markitdown", "opendataloader", "weknoracloud", "mineru", "mineru_cloud", "paddleocr_vl", "paddleocr_vl_cloud"} {
		got := engineOverrides(engine, c, "en")
		if got["mineru_api_key"] != "" && engine != "mineru_cloud" {
			t.Fatalf("MinerU credential sent to %s", engine)
		}
		if got["paddleocr_vl_cloud_token"] != "" && engine != "paddleocr_vl_cloud" {
			t.Fatalf("Paddle credential sent to %s", engine)
		}
	}
}
func TestRedactSecretsAndSignedDownloads(t *testing.T) {
	got := redact("failure secret-123 https://example.org/result?access_token=abc", []string{"secret-123"})
	if strings.Contains(got, "secret-123") || strings.Contains(got, "access_token=abc") {
		t.Fatal(got)
	}
}
