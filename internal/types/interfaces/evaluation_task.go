package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// EvaluationTaskRepository persists tenant-scoped evaluation task snapshots.
type EvaluationTaskRepository interface {
	CreateTask(ctx context.Context, task *types.EvaluationTaskEntity) error
	GetTask(ctx context.Context, tenantID uint64, taskID string) (*types.EvaluationTaskEntity, error)
}
