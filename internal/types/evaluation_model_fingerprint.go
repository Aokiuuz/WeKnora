package types

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

// EvaluationModelBehaviorConfig is the sanitized behavior configuration that
// participates in the model config fingerprint. It deliberately excludes API
// keys, app secrets, app IDs, authentication/trace headers, and URL-embedded
// credentials: the fingerprint explains output behavior, never authentication.
// Behavior-affecting custom header values participate only through SHA-256
// digests so snapshots never carry their plaintext values.
type EvaluationModelBehaviorConfig struct {
	BaseURL              string              `json:"base_url"`
	InterfaceType        string              `json:"interface_type"`
	Provider             string              `json:"provider"`
	ParameterSize        string              `json:"parameter_size"`
	EmbeddingParameters  EmbeddingParameters `json:"embedding_parameters"`
	ExtraConfig          map[string]string   `json:"extra_config"`
	BehaviorHeaderSHA256 map[string]string   `json:"behavior_header_sha256"`
	SupportsVision       bool                `json:"supports_vision"`
	MaxConcurrency       int                 `json:"max_concurrency"`
}

// EvaluationModelSanitizeBaseURL strips userinfo credentials from an upstream
// base URL. The remaining scheme/host/path identifies the endpoint without
// carrying secrets into snapshots or hashes.
func EvaluationModelSanitizeBaseURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Unparseable values carry no structured credentials; keep the raw
		// string so fingerprint drift remains visible.
		return rawURL
	}
	parsed.User = nil
	return parsed.String()
}

// EvaluationModelBehaviorConfigFrom extracts the sanitized behavior
// configuration of one model record.
func EvaluationModelBehaviorConfigFrom(model *Model) *EvaluationModelBehaviorConfig {
	if model == nil {
		return &EvaluationModelBehaviorConfig{
			ExtraConfig:          map[string]string{},
			BehaviorHeaderSHA256: map[string]string{},
		}
	}
	parameters := model.Parameters
	extra := make(map[string]string, len(parameters.ExtraConfig))
	for key, value := range parameters.ExtraConfig {
		if evaluationModelConfigKeyIsSensitive(key) {
			continue
		}
		extra[key] = value
	}
	return &EvaluationModelBehaviorConfig{
		BaseURL:              EvaluationModelSanitizeBaseURL(parameters.BaseURL),
		InterfaceType:        parameters.InterfaceType,
		Provider:             parameters.Provider,
		ParameterSize:        parameters.ParameterSize,
		EmbeddingParameters:  parameters.EmbeddingParameters,
		ExtraConfig:          extra,
		BehaviorHeaderSHA256: ModelBehaviorHeaderSHA256(parameters.CustomHeaders),
		SupportsVision:       parameters.SupportsVision,
		MaxConcurrency:       parameters.MaxConcurrency,
	}
}

// EvaluationModelConfigSHA256 computes the canonical fingerprint of one
// model's sanitized behavior configuration.
func EvaluationModelConfigSHA256(model *Model) string {
	config := EvaluationModelBehaviorConfigFrom(model)
	keys := make([]string, 0, len(config.ExtraConfig))
	for key := range config.ExtraConfig {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(config.ExtraConfig))
	// Map marshaling order is irrelevant: the canonical serializer sorts keys.
	for _, key := range keys {
		ordered[key] = config.ExtraConfig[key]
	}
	encoded := canonicalEvaluationJSONBytes(map[string]any{
		"base_url":       config.BaseURL,
		"interface_type": config.InterfaceType,
		"provider":       config.Provider,
		"parameter_size": config.ParameterSize,
		"embedding_parameters": map[string]any{
			"dimension":                   config.EmbeddingParameters.Dimension,
			"truncate_prompt_tokens":      config.EmbeddingParameters.TruncatePromptTokens,
			"supports_dimension_override": config.EmbeddingParameters.SupportsDimensionOverride,
		},
		"extra_config":           evaluationStringMapToAny(ordered),
		"behavior_header_sha256": evaluationStringMapToAny(config.BehaviorHeaderSHA256),
		"supports_vision":        config.SupportsVision,
		"max_concurrency":        config.MaxConcurrency,
	})
	return "sha256:" + hashEvaluationCanonicalJSON(encoded)
}

// EvaluationModelSnapshotFrom builds the snapshot entry for one model role.
func EvaluationModelSnapshotFrom(model *Model) *EvaluationModelSnapshot {
	if model == nil {
		return nil
	}
	return &EvaluationModelSnapshot{
		ID:            model.ID,
		UpstreamName:  model.Name,
		Type:          string(model.Type),
		Source:        string(model.Source),
		Provider:      model.Parameters.Provider,
		InterfaceType: model.Parameters.InterfaceType,
		ConfigSHA256:  EvaluationModelConfigSHA256(model),
		UpdatedAt:     model.UpdatedAt.UTC(),
	}
}

func evaluationModelConfigKeyIsSensitive(key string) bool {
	parts := splitEvaluationConfigKey(key)
	for _, part := range parts {
		switch part {
		case "secret", "password", "passwd", "authorization", "credential", "credentials":
			return true
		}
	}
	compact := strings.Join(parts, "")
	for _, suffix := range []string{
		"apikey", "appid", "appsecret", "clientid", "clientsecret", "privatekey",
		"secretkey", "accesskey", "accesskeyid", "subscriptionkey", "sessiontoken",
		"accesstoken", "refreshtoken", "idtoken", "authtoken", "bearertoken",
		"password", "credential", "token", "secret",
	} {
		if strings.HasSuffix(compact, suffix) {
			return true
		}
	}
	return false
}

func splitEvaluationConfigKey(key string) []string {
	return strings.FieldsFunc(strings.ToLower(strings.TrimSpace(key)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// ModelBehaviorHeaderSHA256 returns normalized behavior-affecting header names
// mapped to irreversible digests of their configured values. Headers used for
// authentication, request tracing, or transport control are omitted because
// their values do not define model output behavior.
func ModelBehaviorHeaderSHA256(headers map[string]string) map[string]string {
	grouped := make(map[string][]string)
	for key, value := range headers {
		name := strings.ToLower(strings.TrimSpace(key))
		if name == "" || modelHeaderExcludedFromBehavior(name) {
			continue
		}
		grouped[name] = append(grouped[name], value)
	}

	digests := make(map[string]string, len(grouped))
	for name, values := range grouped {
		sort.Strings(values)
		sum := sha256.Sum256([]byte(strings.Join(values, "\x00")))
		digests[name] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return digests
}

func modelHeaderExcludedFromBehavior(name string) bool {
	switch name {
	case "authorization", "proxy-authorization", "api-key", "x-api-key", "x-goog-api-key",
		"cookie", "set-cookie", "content-type", "content-length", "accept-encoding",
		"host", "connection", "transfer-encoding", "traceparent", "tracestate", "baggage":
		return true
	}
	if evaluationModelConfigKeyIsSensitive(name) {
		return true
	}

	parts := splitEvaluationConfigKey(name)
	for _, part := range parts {
		switch part {
		case "auth", "authentication", "secret", "credential", "credentials", "token",
			"trace", "tracing", "requestid", "correlationid", "signature":
			return true
		}
	}
	compact := strings.Join(parts, "")
	return strings.Contains(compact, "requestid") ||
		strings.Contains(compact, "correlationid") ||
		strings.HasPrefix(compact, "xtrace") ||
		strings.HasPrefix(compact, "xb3") ||
		strings.HasPrefix(compact, "xcloudtracecontext")
}

func evaluationStringMapToAny(input map[string]string) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
