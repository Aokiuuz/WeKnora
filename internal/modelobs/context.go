// Package modelobs records secret-free model call facts, usage, latency, and cost.
package modelobs

import "context"

const (
	// PurposeGeneral labels ordinary interactive model calls.
	PurposeGeneral = "general"
	// PurposeEvaluation labels strictly accounted evaluation model calls.
	PurposeEvaluation = "evaluation"
)

type (
	policyContextKey           struct{}
	taskContextKey             struct{}
	applicationCacheContextKey struct{}
)

type callPolicy struct {
	purpose string
	strict  bool
}

// WithPurpose binds a stable purpose and accounting policy to model calls.
func WithPurpose(ctx context.Context, purpose string, strict bool) context.Context {
	if purpose == "" {
		purpose = PurposeGeneral
	}
	return context.WithValue(ctx, policyContextKey{}, callPolicy{purpose: purpose, strict: strict})
}

// WithEvaluationTask binds model calls to one evaluation task.
func WithEvaluationTask(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, taskContextKey{}, taskID)
}

// WithApplicationCacheStatus distinguishes embedding misses and bypasses from provider cache facts.
func WithApplicationCacheStatus(ctx context.Context, status string) context.Context {
	return context.WithValue(ctx, applicationCacheContextKey{}, status)
}

func policyFromContext(ctx context.Context) callPolicy {
	if policy, ok := ctx.Value(policyContextKey{}).(callPolicy); ok {
		return policy
	}
	return callPolicy{purpose: PurposeGeneral}
}
