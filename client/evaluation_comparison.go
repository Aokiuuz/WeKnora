package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type EvaluationComparisonRequest struct {
	TaskIDs        []string `json:"task_ids"`
	BaselineTaskID string   `json:"baseline_task_id,omitempty"`
}

type EvaluationComparisonRun struct {
	TaskID               string           `json:"task_id"`
	Status               EvaluationStatus `json:"status"`
	IsBaseline           bool             `json:"is_baseline"`
	DatasetID            string           `json:"dataset_id"`
	DatasetVersionID     string           `json:"dataset_version_id"`
	VersionNumber        int              `json:"version_number"`
	DatasetContentSHA256 string           `json:"dataset_content_sha256"`
	ProvenanceComplete   bool             `json:"provenance_complete"`
}

type EvaluationComparisonParameterValue struct {
	TaskID  string          `json:"task_id"`
	Missing bool            `json:"missing"`
	Value   json.RawMessage `json:"value,omitempty"`
}

type EvaluationComparisonParameter struct {
	Pointer string                               `json:"pointer"`
	Differ  bool                                 `json:"differ"`
	Values  []EvaluationComparisonParameterValue `json:"values"`
}

type EvaluationComparisonMetricValue struct {
	TaskID         string   `json:"task_id"`
	IsBaseline     bool     `json:"is_baseline"`
	Status         string   `json:"status"`
	Value          *float64 `json:"value"`
	Delta          *float64 `json:"delta"`
	RelativeDelta  *float64 `json:"relative_delta"`
	RelativeReason string   `json:"relative_reason,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

type EvaluationComparisonMetric struct {
	Pointer        string                            `json:"pointer"`
	Key            string                            `json:"key"`
	Version        string                            `json:"version"`
	ConfigSHA256   string                            `json:"config_sha256"`
	Compatible     bool                              `json:"compatible"`
	BaselineTaskID string                            `json:"baseline_task_id"`
	Values         []EvaluationComparisonMetricValue `json:"values"`
}

type EvaluationComparisonResponse struct {
	SchemaVersion  int                             `json:"schema_version"`
	BaselineTaskID string                          `json:"baseline_task_id"`
	Runs           []EvaluationComparisonRun       `json:"runs"`
	Parameters     []EvaluationComparisonParameter `json:"parameters"`
	Metrics        []EvaluationComparisonMetric    `json:"metrics"`
}

type evaluationComparisonEnvelope struct {
	Success bool                          `json:"success"`
	Data    *EvaluationComparisonResponse `json:"data"`
}

// CompareEvaluationRuns decodes the server's authoritative comparison.
func (c *Client) CompareEvaluationRuns(
	ctx context.Context,
	request EvaluationComparisonRequest,
) (*EvaluationComparisonResponse, error) {
	if len(request.TaskIDs) < 2 {
		return nil, errors.New("evaluation comparison requires at least two task IDs")
	}
	response, err := c.doRequest(
		ctx,
		http.MethodPost,
		"/api/v1/evaluation/comparisons",
		request,
		nil,
	)
	if err != nil {
		return nil, err
	}
	var envelope evaluationComparisonEnvelope
	if err := parseResponse(response, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, errors.New("evaluation comparison response is not successful")
	}
	if envelope.Data == nil {
		return nil, errors.New("evaluation comparison response is missing data")
	}
	if envelope.Data.Runs == nil || envelope.Data.Parameters == nil || envelope.Data.Metrics == nil {
		return nil, errors.New("evaluation comparison response has an incomplete data envelope")
	}
	return envelope.Data, nil
}
