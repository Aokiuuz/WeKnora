// Package client provides the implementation for interacting with the WeKnora API.
// Evaluation interfaces start an evaluation task and retrieve its result.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// EvaluationStatus is the numeric lifecycle status returned by the evaluation API.
type EvaluationStatus int

const (
	// EvaluationStatusPending indicates that the task is waiting to run.
	EvaluationStatusPending EvaluationStatus = iota
	// EvaluationStatusRunning indicates that the task is running.
	EvaluationStatusRunning
	// EvaluationStatusSuccess indicates that the task completed successfully.
	EvaluationStatusSuccess
	// EvaluationStatusFailed indicates that the task failed.
	EvaluationStatusFailed
)

// EvaluationTask contains the task state returned by the evaluation API.
type EvaluationTask struct {
	ID        string           `json:"id"`
	TenantID  uint64           `json:"tenant_id"`
	DatasetID string           `json:"dataset_id"`
	StartTime time.Time        `json:"start_time"`
	Status    EvaluationStatus `json:"status"`
	ErrMsg    string           `json:"err_msg,omitempty"`
	Total     int              `json:"total,omitempty"`
	Finished  int              `json:"finished,omitempty"`
}

// UnmarshalJSON validates that an evaluation task contains a numeric status.
func (t *EvaluationTask) UnmarshalJSON(data []byte) error {
	type wireTask struct {
		ID        string            `json:"id"`
		TenantID  uint64            `json:"tenant_id"`
		DatasetID string            `json:"dataset_id"`
		StartTime time.Time         `json:"start_time"`
		Status    *EvaluationStatus `json:"status"`
		ErrMsg    string            `json:"err_msg,omitempty"`
		Total     int               `json:"total,omitempty"`
		Finished  int               `json:"finished,omitempty"`
	}

	var wire wireTask
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode evaluation task: %w", err)
	}
	if wire.Status == nil {
		return errors.New("decode evaluation task: missing numeric status")
	}

	*t = EvaluationTask{
		ID:        wire.ID,
		TenantID:  wire.TenantID,
		DatasetID: wire.DatasetID,
		StartTime: wire.StartTime,
		Status:    *wire.Status,
		ErrMsg:    wire.ErrMsg,
		Total:     wire.Total,
		Finished:  wire.Finished,
	}
	return nil
}

// EvaluationResult contains the task, request parameters, and optional metrics.
type EvaluationResult struct {
	Task   *EvaluationTask         `json:"task"`
	Params json.RawMessage         `json:"params"`
	Metric *EvaluationMetricResult `json:"metric,omitempty"`

	// Experiment is the frozen schema-version-1 experiment manifest; null
	// for pre-M3 tasks. ProvenanceComplete distinguishes full provenance
	// from legacy tasks without a snapshot.
	Experiment         *EvaluationExperimentSnapshot `json:"experiment"`
	ProvenanceComplete bool                          `json:"provenance_complete"`
}

// EvaluationDatasetRef pins the dataset identity inside an experiment snapshot.
type EvaluationDatasetRef struct {
	DatasetID        string `json:"dataset_id"`
	DatasetVersionID string `json:"dataset_version_id"`
	VersionNumber    int    `json:"version_number"`
	ArtifactSHA256   string `json:"artifact_sha256"`
	ContentSHA256    string `json:"content_sha256"`
}

// EvaluationGenerationConfig is the resolved generation section of an
// experiment snapshot, including the seed visibility contract.
type EvaluationGenerationConfig struct {
	Seed         *int   `json:"seed"`
	SeedProvided bool   `json:"seed_provided"`
	SeedSupport  string `json:"seed_support"`
}

// EvaluationExperimentSnapshot is the SDK view of the frozen experiment
// manifest. Less frequently consumed sections stay as raw JSON so the wire
// schema can grow without breaking the SDK.
type EvaluationExperimentSnapshot struct {
	SchemaVersion         int                        `json:"schema_version"`
	Dataset               EvaluationDatasetRef       `json:"dataset"`
	SourceKnowledgeBaseID *string                    `json:"source_knowledge_base_id"`
	Models                json.RawMessage            `json:"models"`
	Configuration         EvaluationExperimentConfig `json:"configuration"`
	MetricPlan            json.RawMessage            `json:"metric_plan"`
	Code                  json.RawMessage            `json:"code"`
	Environment           json.RawMessage            `json:"environment"`
	Reproducibility       json.RawMessage            `json:"reproducibility"`
}

// EvaluationExperimentConfig carries the resolved parameter groups; the
// generation group is structured because seed semantics are part of the M3
// contract.
type EvaluationExperimentConfig struct {
	Retrieval  json.RawMessage            `json:"retrieval"`
	Rerank     json.RawMessage            `json:"rerank"`
	Generation EvaluationGenerationConfig `json:"generation"`
}

// EvaluationMetricResult contains retrieval and generation metrics.
type EvaluationMetricResult struct {
	RetrievalMetrics  EvaluationRetrievalMetrics  `json:"retrieval_metrics"`
	GenerationMetrics EvaluationGenerationMetrics `json:"generation_metrics"`
}

// EvaluationRetrievalMetrics contains retrieval quality metrics.
type EvaluationRetrievalMetrics struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	NDCG3     float64 `json:"ndcg3"`
	NDCG10    float64 `json:"ndcg10"`
	MRR       float64 `json:"mrr"`
	MAP       float64 `json:"map"`
}

// EvaluationGenerationMetrics contains answer generation quality metrics.
type EvaluationGenerationMetrics struct {
	BLEU1  float64 `json:"bleu1"`
	BLEU2  float64 `json:"bleu2"`
	BLEU4  float64 `json:"bleu4"`
	ROUGE1 float64 `json:"rouge1"`
	ROUGE2 float64 `json:"rouge2"`
	ROUGEL float64 `json:"rougel"`
}

// EvaluationRequest contains the parameters accepted by the evaluation API.
type EvaluationRequest struct {
	DatasetID       string `json:"dataset_id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	ChatModelID     string `json:"chat_id"`
	RerankModelID   string `json:"rerank_id"`

	// DatasetVersionID optionally pins one immutable dataset version.
	DatasetVersionID string `json:"dataset_version_id,omitempty"`
	// Seed distinguishes "not provided" (nil) from an explicit seed=0.
	Seed *int `json:"seed,omitempty"`

	// EmbeddingModelID is retained for source compatibility.
	// Deprecated: use KnowledgeBaseID. A non-empty value returns an explicit error.
	EmbeddingModelID string `json:"-"`
}

// ErrEvaluationEmbeddingModelUnsupported reports use of the deprecated embedding model field.
var ErrEvaluationEmbeddingModelUnsupported = errors.New(
	"evaluation request EmbeddingModelID is unsupported; use KnowledgeBaseID",
)

// IsEvaluationSeedUnsupported reports whether err is the evaluation API's
// 422 rejection of an explicit seed against a provider without seed support.
// The task was not created; choosing a seed-capable provider or dropping the
// seed are the only recoveries.
func IsEvaluationSeedUnsupported(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnprocessableEntity
}

// MarshalJSON emits only fields accepted by the evaluation API.
func (r EvaluationRequest) MarshalJSON() ([]byte, error) {
	if r.EmbeddingModelID != "" {
		return nil, ErrEvaluationEmbeddingModelUnsupported
	}

	type wireRequest struct {
		DatasetID        string `json:"dataset_id"`
		KnowledgeBaseID  string `json:"knowledge_base_id"`
		ChatModelID      string `json:"chat_id"`
		RerankModelID    string `json:"rerank_id"`
		DatasetVersionID string `json:"dataset_version_id,omitempty"`
		Seed             *int   `json:"seed,omitempty"`
	}
	return json.Marshal(wireRequest{
		DatasetID:        r.DatasetID,
		KnowledgeBaseID:  r.KnowledgeBaseID,
		ChatModelID:      r.ChatModelID,
		RerankModelID:    r.RerankModelID,
		DatasetVersionID: r.DatasetVersionID,
		Seed:             r.Seed,
	})
}

// EvaluationTaskResponse is the API envelope returned when starting an evaluation.
type EvaluationTaskResponse struct {
	Success bool              `json:"success"`
	Data    *EvaluationResult `json:"data"`
}

// EvaluationResultResponse is the API envelope returned when retrieving an evaluation.
type EvaluationResultResponse struct {
	Success bool              `json:"success"`
	Data    *EvaluationResult `json:"data"`
}

// StartEvaluation starts an evaluation task and returns its nested task state.
func (c *Client) StartEvaluation(ctx context.Context, request *EvaluationRequest) (*EvaluationTask, error) {
	if request == nil {
		return nil, errors.New("evaluation request is required")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/evaluation", request, nil)
	if err != nil {
		return nil, err
	}

	var response EvaluationTaskResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	if err := validateEvaluationResult(response.Data); err != nil {
		return nil, err
	}

	return response.Data.Task, nil
}

// GetEvaluationResult retrieves the nested details of an evaluation task.
func (c *Client) GetEvaluationResult(ctx context.Context, taskID string) (*EvaluationResult, error) {
	queryParams := url.Values{}
	queryParams.Add("task_id", taskID)

	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/evaluation", nil, queryParams)
	if err != nil {
		return nil, err
	}

	var response EvaluationResultResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	if err := validateEvaluationResult(response.Data); err != nil {
		return nil, err
	}

	return response.Data, nil
}

func validateEvaluationResult(result *EvaluationResult) error {
	if result == nil {
		return errors.New("evaluation response is missing data")
	}
	if result.Task == nil {
		return errors.New("evaluation response is missing data.task")
	}
	params := bytes.TrimSpace(result.Params)
	if len(params) == 0 || bytes.Equal(params, []byte("null")) {
		return errors.New("evaluation response is missing data.params")
	}
	return nil
}
