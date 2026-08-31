package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	EvaluationExportSchemaVersion = 1
	EvaluationExportFormatJSON    = "json"
	EvaluationExportFormatCSV     = "csv"
	EvaluationExportPageSize      = 500
	EvaluationExportMaxQuestions  = 50_000
	EvaluationExportMaxBytes      = int64(256 * 1024 * 1024)
)

var (
	ErrEvaluationExportFormatInvalid = errors.New("evaluation export format invalid")
	ErrEvaluationExportTaskConflict  = errors.New("evaluation export task conflict")
	ErrEvaluationExportLimitExceeded = errors.New("evaluation export limit exceeded")
)

func NormalizeEvaluationExportFormat(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	switch format {
	case EvaluationExportFormatJSON, EvaluationExportFormatCSV:
		return format, nil
	default:
		return "", fmt.Errorf("%w: expected json or csv", ErrEvaluationExportFormatInvalid)
	}
}

func IsEvaluationTerminalStatus(status EvaluationStatue) bool {
	switch status {
	case EvaluationStatueSuccess,
		EvaluationStatueFailed,
		EvaluationStatueTimedOut,
		EvaluationStatueInterrupted,
		EvaluationStatueCanceled:
		return true
	default:
		return false
	}
}

type EvaluationPreparedExport struct {
	Path        string
	Filename    string
	ContentType string
	Size        int64
}

type EvaluationExportTask struct {
	ID                   string           `json:"id"`
	DatasetID            string           `json:"dataset_id"`
	DatasetVersionID     *string          `json:"dataset_version_id"`
	DatasetContentSHA256 *string          `json:"dataset_content_sha256"`
	ExperimentSHA256     *string          `json:"experiment_sha256"`
	ProvenanceComplete   bool             `json:"provenance_complete"`
	Status               EvaluationStatue `json:"status"`
	StartTime            time.Time        `json:"start_time"`
	EndTime              *time.Time       `json:"end_time"`
	Total                int              `json:"total"`
	Finished             int              `json:"finished"`
	ErrorMessage         string           `json:"error_message,omitempty"`
	CleanupErrors        JSON             `json:"cleanup_errors"`
}

type EvaluationExportQuestion struct {
	SampleIndex        int    `json:"sample_index"`
	QID                string `json:"qid"`
	Question           string `json:"question"`
	ReferenceAnswer    string `json:"reference_answer"`
	GroundTruthPIDs    JSON   `json:"ground_truth_pids"`
	SearchResults      JSON   `json:"search_results"`
	RerankResults      JSON   `json:"rerank_results"`
	GenerationPIDs     JSON   `json:"generation_pids"`
	GeneratedText      string `json:"generated_text"`
	PerSampleMetrics   JSON   `json:"per_sample_metrics"`
	MetricObservations JSON   `json:"metric_observations"`
	ErrorCode          string `json:"error_code,omitempty"`
	Status             string `json:"status"`
	ResultHash         string `json:"result_hash"`

	RetrievalMs      *int64 `json:"retrieval_ms"`
	RerankMs         *int64 `json:"rerank_ms"`
	GenerationMs     *int64 `json:"generation_ms"`
	TotalMs          *int64 `json:"total_ms"`
	PromptTokens     *int   `json:"prompt_tokens"`
	CompletionTokens *int   `json:"completion_tokens"`
	TotalTokens      *int   `json:"total_tokens"`
}

type EvaluationExportDocument struct {
	SchemaVersion    int                           `json:"schema_version"`
	ExportedAt       time.Time                     `json:"exported_at"`
	Task             EvaluationExportTask          `json:"task"`
	Labels           []string                      `json:"labels"`
	Experiment       *EvaluationExperimentSnapshot `json:"experiment"`
	AggregateMetrics json.RawMessage               `json:"aggregate_metrics"`
	Questions        []EvaluationExportQuestion    `json:"questions"`
}
