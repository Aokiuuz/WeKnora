package types

import (
	"net/url"
	"sort"
	"strings"
	"unicode"
)

// EvaluationModelBehaviorConfig is the sanitized behavior configuration that
// participates in the model config fingerprint. It deliberately excludes API
// keys, app secrets, app IDs, custom headers, and URL-embedded credentials:
// the fingerprint explains output behavior, never authentication. Deployments
// whose routing headers change model behavior can set the non-secret
// ExtraConfig key behavior_revision explicitly.
type EvaluationModelBehaviorConfig struct {
	BaseURL             string              `json:"base_url"`
	InterfaceType       string              `json:"interface_type"`
	Provider            string              `json:"provider"`
	ParameterSize       string              `json:"parameter_size"`
	EmbeddingParameters EmbeddingParameters `json:"embedding_parameters"`
	ExtraConfig         map[string]string   `json:"extra_config"`
	SupportsVision      bool                `json:"supports_vision"`
	MaxConcurrency      int                 `json:"max_concurrency"`
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
		return &EvaluationModelBehaviorConfig{ExtraConfig: map[string]string{}}
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
		BaseURL:             EvaluationModelSanitizeBaseURL(parameters.BaseURL),
		InterfaceType:       parameters.InterfaceType,
		Provider:            parameters.Provider,
		ParameterSize:       parameters.ParameterSize,
		EmbeddingParameters: parameters.EmbeddingParameters,
		ExtraConfig:         extra,
		SupportsVision:      parameters.SupportsVision,
		MaxConcurrency:      parameters.MaxConcurrency,
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
		"extra_config":    evaluationStringMapToAny(ordered),
		"supports_vision": config.SupportsVision,
		"max_concurrency": config.MaxConcurrency,
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

func evaluationStringMapToAny(input map[string]string) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
