package types

import "encoding/json"

// DefaultEvaluationMetricPlan freezes the current twelve quality metrics as

// DefaultEvaluationMetricPlan freezes the current twelve quality metrics as
// one versioned plan. Keys, versions, and configs are owned by M3; the M5
// registry consumes this exact shape through its adapter and must not define
// a parallel default list.
func DefaultEvaluationMetricPlan() (*EvaluationMetricPlanSnapshot, error) {
	type spec struct {
		key     string
		version string
		config  string
	}
	defaults := []spec{
		{"retrieval.precision", "1.0.0", `{}`},
		{"retrieval.recall", "1.0.0", `{}`},
		{"retrieval.ndcg", "1.0.0", `{"k":3}`},
		{"retrieval.ndcg", "1.0.0", `{"k":10}`},
		{"retrieval.mrr", "1.0.0", `{}`},
		{"retrieval.map", "1.0.0", `{}`},
		{"generation.bleu", "1.0.0", `{"n":1}`},
		{"generation.bleu", "1.0.0", `{"n":2}`},
		{"generation.bleu", "1.0.0", `{"n":4}`},
		{"generation.rouge", "1.0.0", `{"variant":"rouge-1"}`},
		{"generation.rouge", "1.0.0", `{"variant":"rouge-2"}`},
		{"generation.rouge", "1.0.0", `{"variant":"rouge-l"}`},
	}
	specs := make([]EvaluationMetricSpecSnapshot, 0, len(defaults))
	for _, entry := range defaults {
		built, err := NewEvaluationMetricSpecSnapshot(entry.key, entry.version, json.RawMessage(entry.config), true)
		if err != nil {
			return nil, err
		}
		specs = append(specs, built)
	}
	return NewEvaluationMetricPlanSnapshot(specs)
}

// EvaluationMetricObservationsFromResult maps one per-sample MetricResult to
// the stable observation list of the frozen plan. Every plan entry gets a
// valid observation; real zero scores stay visible as value=0.
func EvaluationMetricObservationsFromResult(
	plan *EvaluationMetricPlanSnapshot,
	result *MetricResult,
) []EvaluationMetricObservationSnapshot {
	if plan == nil || result == nil {
		return nil
	}
	valueFor := func(instanceID string) (float64, bool) {
		switch {
		case instanceID == EvaluationMetricInstanceID("retrieval.precision", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{}`))):
			return result.RetrievalMetrics.Precision, true
		case instanceID == EvaluationMetricInstanceID("retrieval.recall", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{}`))):
			return result.RetrievalMetrics.Recall, true
		case instanceID == EvaluationMetricInstanceID("retrieval.mrr", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{}`))):
			return result.RetrievalMetrics.MRR, true
		case instanceID == EvaluationMetricInstanceID("retrieval.map", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{}`))):
			return result.RetrievalMetrics.MAP, true
		case instanceID == EvaluationMetricInstanceID("retrieval.ndcg", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"k":3}`))):
			return result.RetrievalMetrics.NDCG3, true
		case instanceID == EvaluationMetricInstanceID("retrieval.ndcg", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"k":10}`))):
			return result.RetrievalMetrics.NDCG10, true
		case instanceID == EvaluationMetricInstanceID("generation.bleu", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"n":1}`))):
			return result.GenerationMetrics.BLEU1, true
		case instanceID == EvaluationMetricInstanceID("generation.bleu", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"n":2}`))):
			return result.GenerationMetrics.BLEU2, true
		case instanceID == EvaluationMetricInstanceID("generation.bleu", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"n":4}`))):
			return result.GenerationMetrics.BLEU4, true
		case instanceID == EvaluationMetricInstanceID("generation.rouge", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"variant":"rouge-1"}`))):
			return result.GenerationMetrics.ROUGE1, true
		case instanceID == EvaluationMetricInstanceID("generation.rouge", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"variant":"rouge-2"}`))):
			return result.GenerationMetrics.ROUGE2, true
		case instanceID == EvaluationMetricInstanceID("generation.rouge", "1.0.0",
			CanonicalEvaluationMetricConfigSHA256(json.RawMessage(`{"variant":"rouge-l"}`))):
			return result.GenerationMetrics.ROUGEL, true
		}
		return 0, false
	}

	observations := make([]EvaluationMetricObservationSnapshot, 0, len(plan.Metrics))
	for _, spec := range plan.Metrics {
		observation := EvaluationMetricObservationSnapshot{InstanceID: spec.InstanceID}
		if value, ok := valueFor(spec.InstanceID); ok {
			observation.Value = &value
			observation.Status = EvaluationMetricObservationValid
		} else {
			observation.Status = EvaluationMetricObservationSkipped
			observation.ErrorCode = "unknown_metric_instance"
		}
		observations = append(observations, observation)
	}
	return observations
}
