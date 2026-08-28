package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrEvaluationTaskNotFound is returned when a task is absent from the
// requested tenant or has been soft-deleted.
var ErrEvaluationTaskNotFound = errors.New("evaluation task not found")

// ErrEvaluationTaskAlreadyExists is returned when a task ID is already persisted.
var ErrEvaluationTaskAlreadyExists = errors.New("evaluation task already exists")

// ErrEvaluationTaskTenantMismatch is returned when the tenant boundary and entity disagree.
var ErrEvaluationTaskTenantMismatch = errors.New("evaluation task tenant mismatch")

type evaluationTaskRepository struct {
	db *gorm.DB
}

// NewEvaluationTaskRepository constructs a database-backed task repository.
func NewEvaluationTaskRepository(db *gorm.DB) interfaces.EvaluationTaskRepository {
	return &evaluationTaskRepository{db: db}
}

// CreateTask persists the initial task snapshot within an explicit tenant boundary.
func (r *evaluationTaskRepository) CreateTask(
	ctx context.Context,
	tenantID uint64,
	task *types.EvaluationTaskEntity,
) error {
	if task == nil {
		return errors.New("create evaluation task: nil task")
	}
	if tenantID == 0 || task.ID == "" || task.TenantID == 0 || task.DatasetID == "" {
		return errors.New("create evaluation task: id, tenant_id, and dataset_id are required")
	}
	if tenantID != task.TenantID {
		return fmt.Errorf(
			"create evaluation task: boundary tenant_id %d, entity tenant_id %d: %w",
			tenantID,
			task.TenantID,
			ErrEvaluationTaskTenantMismatch,
		)
	}
	if task.TemporaryKnowledgeBaseID == "" || task.OwnerID == "" {
		return errors.New("create evaluation task: temporary_kb_id and owner_id are required")
	}
	if task.LeaseExpiresAt == nil || task.LeaseExpiresAt.IsZero() {
		return errors.New("create evaluation task: lease_expires_at is required")
	}
	if err := validateInitialEvaluationTask(task); err != nil {
		return err
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
	normalizeEvaluationTaskTimes(task)
	if task.StartTime.After(task.HeartbeatAt) || task.HeartbeatAt.After(*task.LeaseExpiresAt) {
		return errors.New("create evaluation task: expected start_time <= heartbeat_at <= lease_expires_at")
	}

	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).
		Create(task)
	if result.Error != nil {
		return fmt.Errorf("create evaluation task %s: %w", task.ID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("create evaluation task %s: %w", task.ID, ErrEvaluationTaskAlreadyExists)
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
	normalizeEvaluationTaskTimes(&task)
	return &task, nil
}

func normalizeEvaluationTaskTimes(task *types.EvaluationTaskEntity) {
	task.StartTime = task.StartTime.UTC()
	if task.EndTime != nil {
		endTime := task.EndTime.UTC()
		task.EndTime = &endTime
	}
	if task.LeaseExpiresAt != nil {
		leaseExpiresAt := task.LeaseExpiresAt.UTC()
		task.LeaseExpiresAt = &leaseExpiresAt
	}
	task.HeartbeatAt = task.HeartbeatAt.UTC()
	task.CreatedAt = task.CreatedAt.UTC()
	task.UpdatedAt = task.UpdatedAt.UTC()
}

func validateInitialEvaluationTask(task *types.EvaluationTaskEntity) error {
	if task.Status != types.EvaluationStatuePending {
		return errors.New("create evaluation task: status must be pending")
	}
	if task.EndTime != nil || task.Total != 0 || task.Finished != 0 || task.ErrMsg != "" {
		return errors.New("create evaluation task: terminal and progress fields must be empty")
	}
	if len(task.Metric) != 0 || task.TemporaryKnowledgeID != "" {
		return errors.New("create evaluation task: result and temporary knowledge fields must be empty")
	}
	if task.Version > 1 || task.DeletedAt.Valid {
		return errors.New("create evaluation task: version and deletion fields must describe a new task")
	}
	if len(task.CleanupErrors) != 0 {
		var cleanupErrors []string
		unmarshalErr := json.Unmarshal(task.CleanupErrors, &cleanupErrors)
		if unmarshalErr != nil || cleanupErrors == nil || len(cleanupErrors) != 0 {
			return errors.New("create evaluation task: cleanup_errors must be an empty JSON array")
		}
	}
	if len(task.Params) != 0 {
		var params map[string]json.RawMessage
		if err := json.Unmarshal(task.Params, &params); err != nil || params == nil {
			return errors.New("create evaluation task: params must be a JSON object")
		}
	}
	return nil
}
