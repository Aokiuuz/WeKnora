package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type evaluationComparisonInput struct {
	entity     *types.EvaluationTaskEntity
	experiment *types.EvaluationExperimentSnapshot
	parameters map[string]json.RawMessage
	metrics    map[string]float64
}

func (e *EvaluationService) CompareEvaluations(
	ctx context.Context,
	request types.EvaluationComparisonRequest,
) (*types.EvaluationComparisonResponse, error) {
	ids, err := types.NormalizeEvaluationComparisonTaskIDs(request.TaskIDs)
	if err != nil {
		return nil, err
	}
	baselineID := strings.TrimSpace(request.BaselineTaskID)
	if baselineID == "" {
		baselineID = ids[0]
	}
	if !containsEvaluationTaskID(ids, baselineID) {
		return nil, fmt.Errorf("%w: baseline_task_id must be included in task_ids", types.ErrEvaluationComparisonInvalid)
	}
	tenantID := types.MustTenantIDFromContext(ctx)
	tasksByID, err := e.evaluationTaskRepository.GetTasksByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}

	inputs := make([]evaluationComparisonInput, 0, len(ids))
	for _, taskID := range ids {
		entity, exists := tasksByID[taskID]
		if !exists {
			return nil, fmt.Errorf("%w: %s", types.ErrEvaluationComparisonTaskNotFound, taskID)
		}
		if err := AuthorizeEvaluationTaskForAPIKey(ctx, entity); err != nil {
			if errors.Is(err, interfaces.ErrEvaluationTaskNotFound) {
				return nil, fmt.Errorf("%w: %s", types.ErrEvaluationComparisonTaskNotFound, taskID)
			}
			return nil, err
		}
		experiment, complete, err := decodeEvaluationExperiment(entity)
		if err != nil {
			return nil, fmt.Errorf("%w: task %s experiment: %v", types.ErrEvaluationComparisonDataInvalid, taskID, err)
		}
		if entity.Status != types.EvaluationStatueSuccess {
			return nil, fmt.Errorf("%w: task %s status is %d", types.ErrEvaluationComparisonConflict, taskID, entity.Status)
		}
		if !complete || experiment == nil {
			return nil, fmt.Errorf("%w: task %s provenance is incomplete", types.ErrEvaluationComparisonConflict, taskID)
		}
		parameters, err := types.FlattenEvaluationComparisonParameters(entity.ExperimentSnapshot)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", taskID, err)
		}
		metrics, err := types.FlattenEvaluationNumericMetrics(entity.Metric)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", taskID, err)
		}
		inputs = append(inputs, evaluationComparisonInput{
			entity: entity, experiment: experiment, parameters: parameters, metrics: metrics,
		})
	}
	if err := validateEvaluationComparisonContent(inputs); err != nil {
		return nil, err
	}

	runs := make([]types.EvaluationComparisonRun, 0, len(inputs))
	for _, input := range inputs {
		runs = append(runs, types.EvaluationComparisonRun{
			TaskID:               input.entity.ID,
			Status:               input.entity.Status,
			IsBaseline:           input.entity.ID == baselineID,
			DatasetID:            input.entity.DatasetID,
			DatasetVersionID:     *input.entity.DatasetVersionID,
			VersionNumber:        input.experiment.Dataset.VersionNumber,
			DatasetContentSHA256: *input.entity.DatasetContentSHA256,
			ProvenanceComplete:   true,
		})
	}
	return &types.EvaluationComparisonResponse{
		SchemaVersion:  types.EvaluationComparisonSchemaVersion,
		BaselineTaskID: baselineID,
		Runs:           runs,
		Parameters:     buildEvaluationComparisonParameters(inputs),
		Metrics:        buildEvaluationComparisonMetrics(inputs, baselineID),
	}, nil
}

func containsEvaluationTaskID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func validateEvaluationComparisonContent(inputs []evaluationComparisonInput) error {
	baselineHash := *inputs[0].entity.DatasetContentSHA256
	for _, input := range inputs[1:] {
		if *input.entity.DatasetContentSHA256 != baselineHash {
			return fmt.Errorf(
				"%w: dataset content SHA-256 differs for task %s",
				types.ErrEvaluationComparisonConflict,
				input.entity.ID,
			)
		}
	}
	return nil
}

func buildEvaluationComparisonParameters(inputs []evaluationComparisonInput) []types.EvaluationComparisonParameter {
	pointers := unionEvaluationComparisonPointers(inputs, func(input evaluationComparisonInput) map[string]json.RawMessage {
		return input.parameters
	})
	parameters := make([]types.EvaluationComparisonParameter, 0, len(pointers))
	for _, pointer := range pointers {
		parameter := types.EvaluationComparisonParameter{
			Pointer: pointer,
			Values:  make([]types.EvaluationComparisonParameterValue, 0, len(inputs)),
		}
		reference := ""
		for index, input := range inputs {
			raw, exists := input.parameters[pointer]
			value := types.EvaluationComparisonParameterValue{
				TaskID: input.entity.ID, Missing: !exists, Value: raw,
			}
			canonical := "missing"
			if exists {
				canonical = "value:" + string(raw)
			}
			if index == 0 {
				reference = canonical
			} else if canonical != reference {
				parameter.Differ = true
			}
			parameter.Values = append(parameter.Values, value)
		}
		parameters = append(parameters, parameter)
	}
	return parameters
}

func buildEvaluationComparisonMetrics(
	inputs []evaluationComparisonInput,
	baselineID string,
) []types.EvaluationComparisonMetric {
	pointerSet := make(map[string]struct{})
	for _, input := range inputs {
		for pointer := range input.metrics {
			pointerSet[pointer] = struct{}{}
		}
	}
	pointers := make([]string, 0, len(pointerSet))
	for pointer := range pointerSet {
		pointers = append(pointers, pointer)
	}
	sort.Strings(pointers)

	metrics := make([]types.EvaluationComparisonMetric, 0, len(pointers))
	for _, pointer := range pointers {
		identities := make([]types.EvaluationComparisonMetricIdentity, len(inputs))
		identitiesFound := make([]bool, len(inputs))
		for index, input := range inputs {
			identities[index], identitiesFound[index] = types.EvaluationComparisonMetricIdentityForPath(
				input.experiment,
				pointer,
			)
		}
		identity, identityFound := firstEvaluationMetricIdentity(identities, identitiesFound)
		compatible, reason := compatibleEvaluationMetricIdentities(identities, identitiesFound)
		metric := types.EvaluationComparisonMetric{
			Pointer:        pointer,
			Key:            identity.Key,
			Version:        identity.Version,
			ConfigSHA256:   identity.ConfigSHA256,
			Compatible:     compatible,
			BaselineTaskID: baselineID,
			Values:         make([]types.EvaluationComparisonMetricValue, 0, len(inputs)),
		}
		if !identityFound {
			metric.Key = pointer
		}
		for _, input := range inputs {
			value, exists := input.metrics[pointer]
			wire := types.EvaluationComparisonMetricValue{
				TaskID: input.entity.ID, IsBaseline: input.entity.ID == baselineID,
				Status: types.EvaluationComparisonValueMissing,
			}
			if exists {
				wire.Value = &value
				wire.Status = types.EvaluationComparisonValueValid
				if !compatible {
					wire.Status = types.EvaluationComparisonValueIncompatible
					wire.Reason = reason
				}
			}
			metric.Values = append(metric.Values, wire)
		}
		if compatible {
			completeEvaluationMetricDeltas(&metric)
		}
		metrics = append(metrics, metric)
	}
	return metrics
}

func firstEvaluationMetricIdentity(
	identities []types.EvaluationComparisonMetricIdentity,
	found []bool,
) (types.EvaluationComparisonMetricIdentity, bool) {
	for index, identity := range identities {
		if found[index] {
			return identity, true
		}
	}
	return types.EvaluationComparisonMetricIdentity{}, false
}

func compatibleEvaluationMetricIdentities(
	identities []types.EvaluationComparisonMetricIdentity,
	found []bool,
) (bool, string) {
	reference, ok := firstEvaluationMetricIdentity(identities, found)
	if !ok {
		return false, types.EvaluationComparisonReasonIdentityMissing
	}
	for index, identity := range identities {
		if !found[index] {
			return false, types.EvaluationComparisonReasonIdentityMissing
		}
		if identity != reference {
			return false, types.EvaluationComparisonReasonIdentityDiffers
		}
	}
	return true, ""
}

func completeEvaluationMetricDeltas(metric *types.EvaluationComparisonMetric) {
	var baseline *float64
	for _, value := range metric.Values {
		if value.IsBaseline && value.Status == types.EvaluationComparisonValueValid {
			baseline = value.Value
			break
		}
	}
	if baseline == nil {
		for index := range metric.Values {
			if metric.Values[index].Status == types.EvaluationComparisonValueValid {
				metric.Values[index].Reason = types.EvaluationComparisonReasonBaselineMissing
			}
		}
		return
	}
	for index := range metric.Values {
		value := &metric.Values[index]
		if value.IsBaseline || value.Status != types.EvaluationComparisonValueValid || value.Value == nil {
			continue
		}
		delta := *value.Value - *baseline
		value.Delta = &delta
		if *baseline == 0 {
			value.RelativeReason = types.EvaluationComparisonReasonBaselineZero
			continue
		}
		relative := delta / math.Abs(*baseline)
		value.RelativeDelta = &relative
	}
}

func unionEvaluationComparisonPointers(
	inputs []evaluationComparisonInput,
	selectValues func(evaluationComparisonInput) map[string]json.RawMessage,
) []string {
	set := make(map[string]struct{})
	for _, input := range inputs {
		for pointer := range selectValues(input) {
			set[pointer] = struct{}{}
		}
	}
	pointers := make([]string, 0, len(set))
	for pointer := range set {
		pointers = append(pointers, pointer)
	}
	sort.Strings(pointers)
	return pointers
}
