package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/evaluation/metricregistry"
	"github.com/Tencent/WeKnora/internal/types"
)

const evaluationIntegrityPageSize = types.EvaluationQuestionPageMaxSize

// verifySuccessfulEvaluationResults proves that a successful task is backed
// by one immutable, contiguous row per dataset sample and that the stored
// aggregate is derivable from the frozen metric plan.
func (e *EvaluationService) verifySuccessfulEvaluationResults(
	ctx context.Context,
	tenantID uint64,
	entity *types.EvaluationTaskEntity,
	experiment *types.EvaluationExperimentSnapshot,
) ([]*types.EvaluationQuestionResultEntity, error) {
	if entity == nil || entity.Status != types.EvaluationStatueSuccess {
		return nil, errors.New("verify evaluation results: a successful task is required")
	}
	if e.questionResultRepository == nil {
		return nil, errors.New("verify evaluation results: question result repository is required")
	}
	if entity.Total < 0 || entity.Finished != entity.Total {
		return nil, fmt.Errorf(
			"verify evaluation results: task counters disagree: total=%d finished=%d",
			entity.Total,
			entity.Finished,
		)
	}
	if experiment == nil || experiment.MetricPlan == nil {
		return nil, errors.New("verify evaluation results: frozen metric plan is required")
	}
	registry := e.metricRegistry
	if registry == nil {
		var err error
		registry, err = metricregistry.NewDefaultRegistry()
		if err != nil {
			return nil, fmt.Errorf("verify evaluation results: initialize metric registry: %w", err)
		}
	}
	resolved, err := registry.ResolveSnapshot(experiment.MetricPlan)
	if err != nil {
		return nil, fmt.Errorf("verify evaluation results: resolve frozen metric plan: %w", err)
	}

	rows := make([]*types.EvaluationQuestionResultEntity, 0, entity.Total)
	perSample := make([]*types.MetricResult, 0, entity.Total)
	expectedIndex := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := e.questionResultRepository.ListQuestionResults(
			ctx, tenantID, entity.ID, expectedIndex, evaluationIntegrityPageSize,
		)
		if err != nil {
			return nil, fmt.Errorf("verify evaluation results: list rows: %w", err)
		}
		if len(page) == 0 {
			break
		}
		for _, row := range page {
			if row == nil || row.TenantID != tenantID || row.TaskID != entity.ID {
				return nil, errors.New("verify evaluation results: row ownership is invalid")
			}
			if row.SampleIndex != expectedIndex {
				return nil, fmt.Errorf(
					"verify evaluation results: expected sample_index %d, got %d",
					expectedIndex,
					row.SampleIndex,
				)
			}
			if expectedIndex >= entity.Total {
				return nil, fmt.Errorf("verify evaluation results: row count exceeds total %d", entity.Total)
			}
			input, err := evaluationQuestionResultInputFromEntity(row)
			if err != nil {
				return nil, fmt.Errorf("verify evaluation results: sample %d: %w", row.SampleIndex, err)
			}
			if calculated := types.EvaluationQuestionResultHash(input); calculated != row.ResultHash {
				return nil, fmt.Errorf(
					"verify evaluation results: sample %d result hash mismatch",
					row.SampleIndex,
				)
			}
			if row.Status == types.EvaluationQuestionStatusSuccess {
				if input.PerSampleMetrics == nil {
					return nil, fmt.Errorf(
						"verify evaluation results: sample %d has no metrics",
						row.SampleIndex,
					)
				}
				if err := validateEvaluationMetricEvidence(experiment.MetricPlan, input); err != nil {
					return nil, fmt.Errorf(
						"verify evaluation results: sample %d: %w",
						row.SampleIndex,
						err,
					)
				}
				perSample = append(perSample, input.PerSampleMetrics)
			} else {
				perSample = append(perSample, nil)
			}
			rows = append(rows, row)
			expectedIndex++
		}
		if len(page) < evaluationIntegrityPageSize {
			break
		}
	}
	if expectedIndex != entity.Total || len(rows) != entity.Finished {
		return nil, fmt.Errorf(
			"verify evaluation results: row count=%d total=%d finished=%d",
			len(rows),
			entity.Total,
			entity.Finished,
		)
	}
	recomputed, err := json.Marshal(resolved.Aggregate(perSample))
	if err != nil {
		return nil, fmt.Errorf("verify evaluation results: encode aggregate: %w", err)
	}
	equal, err := evaluationJSONEqual(entity.Metric, recomputed)
	if err != nil {
		return nil, fmt.Errorf("verify evaluation results: decode aggregate: %w", err)
	}
	if !equal {
		return nil, errors.New("verify evaluation results: aggregate metric mismatch")
	}
	return rows, nil
}

func validateEvaluationMetricEvidence(
	plan *types.EvaluationMetricPlanSnapshot,
	input *types.EvaluationQuestionResultInput,
) error {
	if len(input.PerSampleMetrics.Scores) != len(plan.Metrics) || len(input.Observations) != len(plan.Metrics) {
		return fmt.Errorf(
			"metric evidence count mismatch: plan=%d scores=%d observations=%d",
			len(plan.Metrics),
			len(input.PerSampleMetrics.Scores),
			len(input.Observations),
		)
	}
	observations := make(map[string]types.EvaluationMetricObservationSnapshot, len(input.Observations))
	for _, observation := range input.Observations {
		if _, duplicate := observations[observation.InstanceID]; duplicate {
			return fmt.Errorf("duplicate metric observation %s", observation.InstanceID)
		}
		observations[observation.InstanceID] = observation
	}
	fixedCounts := make(map[string]int)
	for _, spec := range plan.Metrics {
		if pointer, ok := types.EvaluationMetricFixedPointer(spec); ok {
			fixedCounts[pointer]++
		}
	}
	for _, spec := range plan.Metrics {
		score, scoreExists := input.PerSampleMetrics.Scores[spec.InstanceID]
		observation, observationExists := observations[spec.InstanceID]
		if !scoreExists || !observationExists {
			return fmt.Errorf("metric evidence lacks %s", spec.InstanceID)
		}
		if score.Status != observation.Status || score.ErrorCode != observation.ErrorCode ||
			!evaluationFloatPointersEqual(score.Value, observation.Value) {
			return fmt.Errorf("metric score and observation disagree for %s", spec.InstanceID)
		}
		switch score.Status {
		case types.EvaluationMetricObservationValid:
			if score.Value == nil || score.ErrorCode != "" {
				return fmt.Errorf("valid metric %s has inconsistent state", spec.InstanceID)
			}
		case types.EvaluationMetricObservationMissing,
			types.EvaluationMetricObservationSkipped,
			types.EvaluationMetricObservationFailed:
			if score.Value != nil {
				return fmt.Errorf("unavailable metric %s carries a numeric value", spec.InstanceID)
			}
		default:
			return fmt.Errorf("metric %s has unknown status %q", spec.InstanceID, score.Status)
		}
		fixedPointer, hasFixedPointer := types.EvaluationMetricFixedPointer(spec)
		if !hasFixedPointer || fixedCounts[fixedPointer] != 1 {
			continue
		}
		fixedValue, _ := types.EvaluationMetricFixedValue(input.PerSampleMetrics, fixedPointer)
		if score.Status == types.EvaluationMetricObservationValid && fixedValue != *score.Value {
			return fmt.Errorf("fixed metric %s disagrees with %s", fixedPointer, spec.InstanceID)
		}
		if score.Status != types.EvaluationMetricObservationValid && fixedValue != 0 {
			return fmt.Errorf("unavailable metric %s has a fixed numeric value", spec.InstanceID)
		}
	}
	return nil
}

func evaluationFloatPointersEqual(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func evaluationQuestionResultInputFromEntity(
	row *types.EvaluationQuestionResultEntity,
) (*types.EvaluationQuestionResultInput, error) {
	if row == nil {
		return nil, errors.New("question result row is required")
	}
	input := &types.EvaluationQuestionResultInput{
		SampleIndex: row.SampleIndex, QID: row.QID, Question: row.Question,
		ReferenceAnswer: row.ReferenceAnswer, GeneratedText: row.GeneratedText,
		ErrorCode: row.ErrorCode, RetrievalMs: row.RetrievalMs, RerankMs: row.RerankMs,
		GenerationMs: row.GenerationMs, TotalMs: row.TotalMs, PromptTokens: row.PromptTokens,
		CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		UsageReported: row.UsageReported, Status: row.Status,
	}
	decoders := []struct {
		name  string
		raw   types.JSON
		value any
	}{
		{name: "ground_truth_pids", raw: row.GroundTruthPIDs, value: &input.GroundTruthPIDs},
		{name: "search_results", raw: row.SearchResults, value: &input.SearchResults},
		{name: "rerank_results", raw: row.RerankResults, value: &input.RerankResults},
		{name: "generation_pids", raw: row.GenerationPIDs, value: &input.GenerationPIDs},
		{name: "per_sample_metrics", raw: row.PerSampleMetrics, value: &input.PerSampleMetrics},
		{name: "metric_observations", raw: row.MetricObservations, value: &input.Observations},
	}
	for _, decoder := range decoders {
		if len(bytes.TrimSpace(decoder.raw)) == 0 || !json.Valid(decoder.raw) {
			return nil, fmt.Errorf("%s is not valid JSON", decoder.name)
		}
		if err := json.Unmarshal(decoder.raw, decoder.value); err != nil {
			return nil, fmt.Errorf("decode %s: %w", decoder.name, err)
		}
	}
	return input, nil
}

func evaluationJSONEqual(left, right []byte) (bool, error) {
	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false, err
	}
	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false, err
	}
	leftCanonical, err := json.Marshal(leftValue)
	if err != nil {
		return false, err
	}
	rightCanonical, err := json.Marshal(rightValue)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftCanonical, rightCanonical), nil
}
