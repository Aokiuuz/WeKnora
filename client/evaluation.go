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
	// EvaluationStatusTimedOut indicates that the task exceeded its deadline.
	EvaluationStatusTimedOut
	// EvaluationStatusInterrupted indicates that task execution was interrupted.
	EvaluationStatusInterrupted
	// EvaluationStatusCanceled indicates that the task was canceled by request.
	EvaluationStatusCanceled
)

// EvaluationTask contains the task state returned by the evaluation API.
type EvaluationTask struct {
	ID        string           `json:"id"`
	TenantID  uint64           `json:"tenant_id"`
	DatasetID string           `json:"dataset_id"`
	StartTime time.Time        `json:"start_time"`
	EndTime   *time.Time       `json:"end_time,omitempty"`
	Status    EvaluationStatus `json:"status"`
	ErrMsg    string           `json:"err_msg,omitempty"`

	CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`

	CleanupErrors []string `json:"cleanup_errors,omitempty"`

	Total    int `json:"total,omitempty"`
	Finished int `json:"finished,omitempty"`
}

// UnmarshalJSON validates that an evaluation task contains a numeric status.
func (t *EvaluationTask) UnmarshalJSON(data []byte) error {
	type wireTask struct {
		ID        string            `json:"id"`
		TenantID  uint64            `json:"tenant_id"`
		DatasetID string            `json:"dataset_id"`
		StartTime time.Time         `json:"start_time"`
		EndTime   *time.Time        `json:"end_time,omitempty"`
		Status    *EvaluationStatus `json:"status"`
		ErrMsg    string            `json:"err_msg,omitempty"`

		CancelRequestedAt *time.Time `json:"cancel_requested_at,omitempty"`

		CleanupErrors []string `json:"cleanup_errors,omitempty"`

		Total    int `json:"total,omitempty"`
		Finished int `json:"finished,omitempty"`
	}

	var wire wireTask
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode evaluation task: %w", err)
	}
	if wire.Status == nil {
		return errors.New("decode evaluation task: missing numeric status")
	}
	if !wire.Status.valid() {
		return fmt.Errorf("decode evaluation task: unknown numeric status %d", *wire.Status)
	}

	*t = EvaluationTask{
		ID:                wire.ID,
		TenantID:          wire.TenantID,
		DatasetID:         wire.DatasetID,
		StartTime:         wire.StartTime,
		EndTime:           wire.EndTime,
		Status:            *wire.Status,
		ErrMsg:            wire.ErrMsg,
		CancelRequestedAt: wire.CancelRequestedAt,
		CleanupErrors:     wire.CleanupErrors,
		Total:             wire.Total,
		Finished:          wire.Finished,
	}
	return nil
}

func (s EvaluationStatus) valid() bool {
	return s >= EvaluationStatusPending && s <= EvaluationStatusCanceled
}

// EvaluationResult contains the task, request parameters, and optional metrics.
type EvaluationResult struct {
	Task   *EvaluationTask         `json:"task"`
	Params json.RawMessage         `json:"params"`
	Metric *EvaluationMetricResult `json:"metric,omitempty"`
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

	// EmbeddingModelID is retained for source compatibility.
	// Deprecated: use KnowledgeBaseID. A non-empty value returns an explicit error.
	EmbeddingModelID string `json:"-"`
}

// ErrEvaluationEmbeddingModelUnsupported reports use of the deprecated embedding model field.
var ErrEvaluationEmbeddingModelUnsupported = errors.New(
	"evaluation request EmbeddingModelID is unsupported; use KnowledgeBaseID",
)

// MarshalJSON emits only fields accepted by the evaluation API.
func (r EvaluationRequest) MarshalJSON() ([]byte, error) {
	if r.EmbeddingModelID != "" {
		return nil, ErrEvaluationEmbeddingModelUnsupported
	}

	type wireRequest struct {
		DatasetID       string `json:"dataset_id"`
		KnowledgeBaseID string `json:"knowledge_base_id"`
		ChatModelID     string `json:"chat_id"`
		RerankModelID   string `json:"rerank_id"`
	}
	return json.Marshal(wireRequest{
		DatasetID:       r.DatasetID,
		KnowledgeBaseID: r.KnowledgeBaseID,
		ChatModelID:     r.ChatModelID,
		RerankModelID:   r.RerankModelID,
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

// DeleteEvaluation soft-deletes one terminal evaluation task. Missing and
// already deleted tasks succeed; the server rejects active tasks with 409.
func (c *Client) DeleteEvaluation(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("evaluation task ID is required")
	}

	resp, err := c.doRequest(
		ctx,
		http.MethodDelete,
		"/api/v1/evaluation/"+url.PathEscape(taskID),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// EvaluationListOptions carries the optional list filters: a numeric status,
// a bounded page size, and the opaque keyset cursor from the previous page.
type EvaluationListOptions struct {
	Status   *EvaluationStatus
	PageSize int
	Cursor   string
}

// EvaluationTaskPage contains one keyset page and the next cursor.
type EvaluationTaskPage struct {
	Items      []*EvaluationTask
	NextCursor string
}

type evaluationListData struct {
	Items      []*EvaluationTask `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

// EvaluationListResponse is the API envelope returned when listing tasks.
type EvaluationListResponse struct {
	Success bool                `json:"success"`
	Data    *evaluationListData `json:"data"`
}

// ListEvaluations returns one keyset page of evaluation tasks ordered by
// (start_time DESC, id DESC). The response must contain the nested data
// object; a missing one is an error instead of a silent empty page.
func (c *Client) ListEvaluations(ctx context.Context, options *EvaluationListOptions) (*EvaluationTaskPage, error) {
	queryParams := url.Values{}
	if options != nil {
		if options.Status != nil {
			queryParams.Add("status", fmt.Sprintf("%d", *options.Status))
		}
		if options.PageSize > 0 {
			queryParams.Add("page_size", fmt.Sprintf("%d", options.PageSize))
		}
		if options.Cursor != "" {
			queryParams.Add("cursor", options.Cursor)
		}
	}

	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/evaluation/tasks", nil, queryParams)
	if err != nil {
		return nil, err
	}

	var response EvaluationListResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}
	if !response.Success {
		return nil, errors.New("evaluation list response is not successful")
	}
	if response.Data == nil {
		return nil, errors.New("evaluation list response is missing data")
	}
	items := response.Data.Items
	if items == nil {
		items = []*EvaluationTask{}
	}
	return &EvaluationTaskPage{
		Items:      items,
		NextCursor: response.Data.NextCursor,
	}, nil
}

// CancelEvaluation requests cancellation of an evaluation task and returns
// its nested state. Requesting cancel twice or on a terminal task returns the
// current task unchanged.
func (c *Client) CancelEvaluation(ctx context.Context, taskID string) (*EvaluationResult, error) {
	if taskID == "" {
		return nil, errors.New("evaluation task ID is required")
	}

	resp, err := c.doRequest(
		ctx,
		http.MethodPost,
		"/api/v1/evaluation/"+url.PathEscape(taskID)+"/cancel",
		nil,
		nil,
	)
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
