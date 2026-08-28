package types

import "encoding/json"

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
