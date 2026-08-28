package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// ErrEvaluationTaskNotFound is returned when a task is absent from the
// requested tenant or has been soft-deleted.
var ErrEvaluationTaskNotFound = errors.New("evaluation task not found")

type evaluationTaskRepository struct {
	db *gorm.DB
}

// NewEvaluationTaskRepository constructs a database-backed task repository.
func NewEvaluationTaskRepository(db *gorm.DB) interfaces.EvaluationTaskRepository {
	return &evaluationTaskRepository{db: db}
}

// CreateTask persists the initial task snapshot.
func (r *evaluationTaskRepository) CreateTask(ctx context.Context, task *types.EvaluationTaskEntity) error {
	if task == nil {
		return errors.New("create evaluation task: nil task")
	}
	if task.ID == "" || task.TenantID == 0 || task.DatasetID == "" {
		return errors.New("create evaluation task: id, tenant_id, and dataset_id are required")
	}
	if task.TemporaryKnowledgeBaseID == "" || task.OwnerID == "" {
		return errors.New("create evaluation task: temporary_kb_id and owner_id are required")
	}
	if task.LeaseExpiresAt.IsZero() {
		return errors.New("create evaluation task: lease_expires_at is required")
	}

	now := time.Now().UTC()
	if task.StartTime.IsZero() {
		task.StartTime = now
	}
	if task.HeartbeatAt.IsZero() {
		task.HeartbeatAt = task.StartTime
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	if task.Version == 0 {
		task.Version = 1
	}
	if len(task.CleanupErrors) == 0 {
		task.CleanupErrors = types.JSON(`[]`)
	}
	if len(task.Params) == 0 {
		task.Params = types.JSON(`{}`)
	}

	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return fmt.Errorf("create evaluation task %s: %w", task.ID, err)
	}
	return nil
}

// GetTask returns one task only when it belongs to the requested tenant.
func (r *evaluationTaskRepository) GetTask(
	ctx context.Context,
	tenantID uint64,
	taskID string,
) (*types.EvaluationTaskEntity, error) {
	var task types.EvaluationTaskEntity
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, taskID).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrEvaluationTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get evaluation task %s: %w", taskID, err)
	}
	return &task, nil
}
