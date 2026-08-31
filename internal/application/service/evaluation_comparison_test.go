package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const evaluationComparisonContentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func comparisonTaskFixture(t *testing.T, taskID string, topK int, precision float64) *types.EvaluationTaskEntity {
	t.Helper()
	plan, err := types.DefaultEvaluationMetricPlan()
	require.NoError(t, err)
	sourceID := "kb-source"
	experiment := &types.EvaluationExperimentSnapshot{
		SchemaVersion: types.EvaluationExperimentSchemaVersion,
		Dataset: types.EvaluationDatasetSnapshot{
			DatasetID: "dataset-a", DatasetVersionID: "version-a", VersionNumber: 3,
			ContentSHA256: evaluationComparisonContentHash,
		},
		SourceKnowledgeBaseID: &sourceID,
		Models: types.EvaluationModelSetSnapshot{
			Chat: &types.EvaluationModelSnapshot{ID: "chat-a"},
		},
		Configuration: types.EvaluationConfigurationSnapshot{
			Retrieval: types.EvaluationRetrievalSnapshot{EmbeddingTopK: topK},
			Generation: types.EvaluationGenerationSnapshot{
				Seed: intPointer(42), SeedProvided: true, SeedSupport: types.EvaluationSeedSupportApplied,
			},
		},
		MetricPlan: plan,
	}
	snapshot, err := experiment.CanonicalJSON()
	require.NoError(t, err)
	versionID := "version-a"
	experimentHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	return &types.EvaluationTaskEntity{
		ID: taskID, TenantID: 7, DatasetID: "dataset-a", Status: types.EvaluationStatueSuccess,
		StartTime:        time.Date(2026, 8, 31, 11, 0, 0, 0, time.UTC),
		DatasetVersionID: &versionID, DatasetContentSHA256: stringPointer(evaluationComparisonContentHash),
		ExperimentSnapshot: snapshot, ExperimentSHA256: &experimentHash,
		Metric: types.JSON(`{"retrieval_metrics":{"precision":` + jsonNumber(precision) + `}}`),
	}
}

func intPointer(value int) *int { return &value }

func stringPointer(value string) *string { return &value }

func jsonNumber(value float64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func comparisonServiceContext() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
}

func TestCompareEvaluationsBuildsStableParameterAndMetricDeltas(t *testing.T) {
	repo := newFakeEvaluationTaskRepository()
	repo.register(comparisonTaskFixture(t, "task-a", 5, 0.5))
	repo.register(comparisonTaskFixture(t, "task-b", 10, 0.6))
	svc := &EvaluationService{evaluationTaskRepository: repo}

	response, err := svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-b"},
	})
	require.NoError(t, err)
	require.Equal(t, "task-a", response.BaselineTaskID)
	require.True(t, response.Runs[0].IsBaseline)
	require.Equal(t, 1, repo.countCalls("GetTasksByIDs"))

	topK := comparisonParameterByPointer(t, response, "/configuration/retrieval/embedding_top_k")
	require.True(t, topK.Differ)
	require.JSONEq(t, `5`, string(topK.Values[0].Value))
	require.JSONEq(t, `10`, string(topK.Values[1].Value))

	precision := comparisonMetricByPointer(t, response, "/retrieval_metrics/precision")
	require.True(t, precision.Compatible)
	require.Equal(t, "retrieval.precision", precision.Key)
	require.Nil(t, precision.Values[0].Delta)
	require.NotNil(t, precision.Values[1].Delta)
	require.InDelta(t, 0.1, *precision.Values[1].Delta, 1e-12)
	require.NotNil(t, precision.Values[1].RelativeDelta)
	require.InDelta(t, 0.2, *precision.Values[1].RelativeDelta, 1e-12)
}

func TestCompareEvaluationsKeepsRelativeDeltaNullForZeroBaseline(t *testing.T) {
	repo := newFakeEvaluationTaskRepository()
	repo.register(comparisonTaskFixture(t, "task-a", 5, 0))
	repo.register(comparisonTaskFixture(t, "task-b", 5, 0.25))
	svc := &EvaluationService{evaluationTaskRepository: repo}

	response, err := svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-b"},
	})
	require.NoError(t, err)
	precision := comparisonMetricByPointer(t, response, "/retrieval_metrics/precision")
	require.NotNil(t, precision.Values[0].Value)
	require.Zero(t, *precision.Values[0].Value)
	require.NotNil(t, precision.Values[1].Delta)
	require.Nil(t, precision.Values[1].RelativeDelta)
	require.Equal(t, types.EvaluationComparisonReasonBaselineZero, precision.Values[1].RelativeReason)
}

func TestCompareEvaluationsMarksFrozenMetricIdentityMismatch(t *testing.T) {
	repo := newFakeEvaluationTaskRepository()
	baseline := comparisonTaskFixture(t, "task-a", 5, 0.5)
	other := comparisonTaskFixture(t, "task-b", 5, 0.6)
	var experiment types.EvaluationExperimentSnapshot
	require.NoError(t, json.Unmarshal(other.ExperimentSnapshot, &experiment))
	for index := range experiment.MetricPlan.Metrics {
		if experiment.MetricPlan.Metrics[index].Key == "retrieval.precision" {
			experiment.MetricPlan.Metrics[index].Version = "2.0.0"
			experiment.MetricPlan.Metrics[index].InstanceID = types.EvaluationMetricInstanceID(
				experiment.MetricPlan.Metrics[index].Key,
				experiment.MetricPlan.Metrics[index].Version,
				experiment.MetricPlan.Metrics[index].ConfigSHA256,
			)
		}
	}
	encoded, err := experiment.CanonicalJSON()
	require.NoError(t, err)
	other.ExperimentSnapshot = encoded
	repo.register(baseline)
	repo.register(other)
	svc := &EvaluationService{evaluationTaskRepository: repo}

	response, err := svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-b"},
	})
	require.NoError(t, err)
	precision := comparisonMetricByPointer(t, response, "/retrieval_metrics/precision")
	require.False(t, precision.Compatible)
	require.Equal(t, types.EvaluationComparisonValueIncompatible, precision.Values[0].Status)
	require.Equal(t, types.EvaluationComparisonReasonIdentityDiffers, precision.Values[1].Reason)
	require.Nil(t, precision.Values[1].Delta)
}

func TestCompareEvaluationsRejectsMissingCrossTenantAndIncompatibleRuns(t *testing.T) {
	repo := newFakeEvaluationTaskRepository()
	repo.register(comparisonTaskFixture(t, "task-a", 5, 0.5))
	hidden := comparisonTaskFixture(t, "task-hidden", 5, 0.6)
	hidden.TenantID = 8
	repo.register(hidden)
	svc := &EvaluationService{evaluationTaskRepository: repo}

	_, err := svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-hidden"},
	})
	require.ErrorIs(t, err, types.ErrEvaluationComparisonTaskNotFound)

	failed := comparisonTaskFixture(t, "task-failed", 5, 0.6)
	failed.Status = types.EvaluationStatueFailed
	repo.register(failed)
	_, err = svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-failed"},
	})
	require.ErrorIs(t, err, types.ErrEvaluationComparisonConflict)

	incomplete := comparisonTaskFixture(t, "task-incomplete", 5, 0.6)
	incomplete.ExperimentSHA256 = nil
	repo.register(incomplete)
	_, err = svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-incomplete"},
	})
	require.ErrorIs(t, err, types.ErrEvaluationComparisonConflict)

	mismatch := comparisonTaskFixture(t, "task-mismatch", 5, 0.6)
	mismatch.DatasetContentSHA256 = stringPointer("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	repo.register(mismatch)
	_, err = svc.CompareEvaluations(comparisonServiceContext(), types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-mismatch"},
	})
	require.ErrorIs(t, err, types.ErrEvaluationComparisonConflict)
}

func TestCompareEvaluationsEnforcesAPIKeySourceKnowledgeBaseScope(t *testing.T) {
	repo := newFakeEvaluationTaskRepository()
	repo.register(comparisonTaskFixture(t, "task-a", 5, 0.5))
	repo.register(comparisonTaskFixture(t, "task-b", 5, 0.6))
	svc := &EvaluationService{evaluationTaskRepository: repo}
	ctx := types.WithTenantAPIKeyScope(comparisonServiceContext(), types.TenantAPIKeyScope{
		KnowledgeBaseIDs: []string{"kb-other"},
	})
	_, err := svc.CompareEvaluations(ctx, types.EvaluationComparisonRequest{
		TaskIDs: []string{"task-a", "task-b"},
	})
	require.ErrorIs(t, err, types.ErrEvaluationComparisonTaskNotFound)
}

func comparisonParameterByPointer(
	t *testing.T,
	response *types.EvaluationComparisonResponse,
	pointer string,
) types.EvaluationComparisonParameter {
	t.Helper()
	for _, parameter := range response.Parameters {
		if parameter.Pointer == pointer {
			return parameter
		}
	}
	t.Fatalf("comparison parameter %s was not found", pointer)
	return types.EvaluationComparisonParameter{}
}

func comparisonMetricByPointer(
	t *testing.T,
	response *types.EvaluationComparisonResponse,
	pointer string,
) types.EvaluationComparisonMetric {
	t.Helper()
	for _, metric := range response.Metrics {
		if metric.Pointer == pointer {
			return metric
		}
	}
	t.Fatalf("comparison metric %s was not found", pointer)
	return types.EvaluationComparisonMetric{}
}

func TestNormalizeComparisonDeduplicatesBeforeLimitResult(t *testing.T) {
	ids, err := types.NormalizeEvaluationComparisonTaskIDs([]string{"task-a", "task-b", "task-a"})
	require.NoError(t, err)
	assert.Equal(t, []string{"task-a", "task-b"}, ids)
}
