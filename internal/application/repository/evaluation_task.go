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

var (
	// ErrEvaluationTaskNotFound aliases the shared repository not-found sentinel.
	ErrEvaluationTaskNotFound = interfaces.ErrEvaluationTaskNotFound
	// ErrEvaluationTaskAlreadyExists aliases the shared repository duplicate sentinel.
	ErrEvaluationTaskAlreadyExists = interfaces.ErrEvaluationTaskAlreadyExists
	// ErrEvaluationTaskTenantMismatch aliases the shared repository tenant sentinel.
	ErrEvaluationTaskTenantMismatch = interfaces.ErrEvaluationTaskTenantMismatch
	// ErrEvaluationTaskOwnerConflict aliases the shared repository owner sentinel.
	ErrEvaluationTaskOwnerConflict = interfaces.ErrEvaluationTaskOwnerConflict
	// ErrEvaluationTaskVersionConflict aliases the shared repository version sentinel.
	ErrEvaluationTaskVersionConflict = interfaces.ErrEvaluationTaskVersionConflict
	// ErrEvaluationTaskStateConflict aliases the shared repository state sentinel.
	ErrEvaluationTaskStateConflict = interfaces.ErrEvaluationTaskStateConflict
)

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

// TryStartTask moves one lease-valid pending task to Running for its current owner.
func (r *evaluationTaskRepository) TryStartTask(
	ctx context.Context,
	command types.EvaluationTaskStartCommand,
) (*types.EvaluationTaskEntity, error) {
	if err := validateEvaluationTaskMutationIdentity(
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
	); err != nil {
		return nil, fmt.Errorf("start evaluation task: %w", err)
	}
	if command.Now.IsZero() || command.LeaseExpiresAt.IsZero() {
		return nil, errors.New("start evaluation task: now and lease_expires_at are required")
	}
	command.Now = command.Now.UTC()
	command.LeaseExpiresAt = command.LeaseExpiresAt.UTC()
	if !command.LeaseExpiresAt.After(command.Now) {
		return nil, errors.New("start evaluation task: lease_expires_at must be after now")
	}

	return r.updateEvaluationTask(
		ctx,
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
		[]types.EvaluationStatue{types.EvaluationStatuePending},
		"lease_expires_at > ? AND start_time <= ?",
		[]any{command.Now, command.Now},
		map[string]any{
			"status":         types.EvaluationStatueRunning,
			"end_time":       nil,
			"err_msg":        "",
			"cleanup_errors": types.JSON(`[]`),
			"heartbeat_at": gorm.Expr(
				"CASE WHEN heartbeat_at > ? THEN heartbeat_at ELSE ? END",
				command.Now, command.Now,
			),
			"lease_expires_at": gorm.Expr(
				"CASE WHEN lease_expires_at > ? THEN lease_expires_at ELSE ? END",
				command.LeaseExpiresAt, command.LeaseExpiresAt,
			),
			"updated_at": gorm.Expr(
				"CASE WHEN updated_at > ? THEN updated_at ELSE ? END",
				command.Now, command.Now,
			),
			"version": gorm.Expr("version + 1"),
		},
		func(task *types.EvaluationTaskEntity) bool {
			return task.Status == types.EvaluationStatuePending &&
				task.LeaseExpiresAt != nil && task.LeaseExpiresAt.After(command.Now) &&
				!task.StartTime.After(command.Now)
		},
		"start",
	)
}

// PublishProgress atomically publishes a complete progress snapshot and renews its lease.
func (r *evaluationTaskRepository) PublishProgress(
	ctx context.Context,
	command types.EvaluationTaskProgressCommand,
) (*types.EvaluationTaskEntity, error) {
	if err := validateEvaluationTaskMutationIdentity(
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
	); err != nil {
		return nil, fmt.Errorf("publish evaluation progress: %w", err)
	}
	if command.Now.IsZero() || command.LeaseExpiresAt.IsZero() {
		return nil, errors.New("publish evaluation progress: now and lease_expires_at are required")
	}
	command.Now = command.Now.UTC()
	command.LeaseExpiresAt = command.LeaseExpiresAt.UTC()
	if !command.LeaseExpiresAt.After(command.Now) {
		return nil, errors.New("publish evaluation progress: lease_expires_at must be after now")
	}
	if command.Total < 0 || command.Finished < 0 || command.Finished > command.Total {
		return nil, errors.New("publish evaluation progress: expected 0 <= finished <= total")
	}
	if err := validateEvaluationTaskJSONObject(command.Metric, true); err != nil {
		return nil, fmt.Errorf("publish evaluation progress: metric: %w", err)
	}

	return r.updateEvaluationTask(
		ctx,
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
		[]types.EvaluationStatue{types.EvaluationStatueRunning},
		"(total = 0 OR total = ?) AND finished <= ?",
		[]any{command.Total, command.Finished},
		map[string]any{
			"total":    command.Total,
			"finished": command.Finished,
			"metric":   command.Metric,
			"heartbeat_at": gorm.Expr(
				"CASE WHEN heartbeat_at > ? THEN heartbeat_at ELSE ? END",
				command.Now, command.Now,
			),
			"lease_expires_at": gorm.Expr(
				"CASE WHEN lease_expires_at > ? THEN lease_expires_at ELSE ? END",
				command.LeaseExpiresAt, command.LeaseExpiresAt,
			),
			"updated_at": gorm.Expr(
				"CASE WHEN updated_at > ? THEN updated_at ELSE ? END",
				command.Now, command.Now,
			),
			"version": gorm.Expr("version + 1"),
		},
		func(task *types.EvaluationTaskEntity) bool {
			return task.Status == types.EvaluationStatueRunning &&
				(task.Total == 0 || task.Total == command.Total) &&
				task.Finished <= command.Finished
		},
		"publish progress for",
	)
}

// RecordTemporaryKnowledge records resource ownership once for a running task.
func (r *evaluationTaskRepository) RecordTemporaryKnowledge(
	ctx context.Context,
	command types.EvaluationTaskKnowledgeCommand,
) (*types.EvaluationTaskEntity, error) {
	if err := validateEvaluationTaskMutationIdentity(
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
	); err != nil {
		return nil, fmt.Errorf("record evaluation temporary knowledge: %w", err)
	}
	if command.TemporaryKnowledgeID == "" || command.UpdatedAt.IsZero() {
		return nil, errors.New("record evaluation temporary knowledge: knowledge ID and updated_at are required")
	}
	command.UpdatedAt = command.UpdatedAt.UTC()

	return r.updateEvaluationTask(
		ctx,
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
		[]types.EvaluationStatue{types.EvaluationStatueRunning},
		"(temporary_knowledge_id IS NULL OR temporary_knowledge_id = '')",
		nil,
		map[string]any{
			"temporary_knowledge_id": command.TemporaryKnowledgeID,
			"updated_at": gorm.Expr(
				"CASE WHEN updated_at > ? THEN updated_at ELSE ? END",
				command.UpdatedAt, command.UpdatedAt,
			),
			"version": gorm.Expr("version + 1"),
		},
		func(task *types.EvaluationTaskEntity) bool {
			return task.Status == types.EvaluationStatueRunning && task.TemporaryKnowledgeID == ""
		},
		"record temporary knowledge for",
	)
}

// PublishTerminal atomically stores a stable terminal snapshot and releases the lease.
func (r *evaluationTaskRepository) PublishTerminal(
	ctx context.Context,
	command types.EvaluationTaskTerminalCommand,
) (*types.EvaluationTaskEntity, error) {
	if err := validateEvaluationTaskMutationIdentity(
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
	); err != nil {
		return nil, fmt.Errorf("publish evaluation terminal state: %w", err)
	}
	if command.EndTime.IsZero() {
		return nil, errors.New("publish evaluation terminal state: end_time is required")
	}
	command.EndTime = command.EndTime.UTC()
	switch command.Status {
	case types.EvaluationStatueSuccess:
		if command.ErrMsg != "" {
			return nil, errors.New("publish evaluation terminal state: successful task must not have err_msg")
		}
	case types.EvaluationStatueFailed, types.EvaluationStatueTimedOut:
		if command.ErrMsg == "" {
			return nil, errors.New("publish evaluation terminal state: failed or timed out task requires err_msg")
		}
	default:
		return nil, errors.New("publish evaluation terminal state: unsupported terminal status")
	}
	if len(command.CleanupErrors) == 0 {
		command.CleanupErrors = types.JSON(`[]`)
	}
	if err := validateEvaluationTaskStringArray(command.CleanupErrors); err != nil {
		return nil, fmt.Errorf("publish evaluation terminal state: cleanup_errors: %w", err)
	}
	if err := validateEvaluationTaskJSONObject(command.Metric, true); err != nil {
		return nil, fmt.Errorf("publish evaluation terminal state: metric: %w", err)
	}

	return r.updateEvaluationTask(
		ctx,
		command.TenantID,
		command.TaskID,
		command.OwnerID,
		command.ExpectedVersion,
		[]types.EvaluationStatue{types.EvaluationStatuePending, types.EvaluationStatueRunning},
		"start_time <= ? AND heartbeat_at <= ?",
		[]any{command.EndTime, command.EndTime},
		map[string]any{
			"status":           command.Status,
			"end_time":         command.EndTime,
			"err_msg":          command.ErrMsg,
			"cleanup_errors":   command.CleanupErrors,
			"metric":           command.Metric,
			"lease_expires_at": nil,
			"updated_at": gorm.Expr(
				"CASE WHEN updated_at > ? THEN updated_at ELSE ? END",
				command.EndTime, command.EndTime,
			),
			"version": gorm.Expr("version + 1"),
		},
		func(task *types.EvaluationTaskEntity) bool {
			return (task.Status == types.EvaluationStatuePending || task.Status == types.EvaluationStatueRunning) &&
				!task.StartTime.After(command.EndTime) && !task.HeartbeatAt.After(command.EndTime)
		},
		"publish terminal state for",
	)
}

func (r *evaluationTaskRepository) updateEvaluationTask(
	ctx context.Context,
	tenantID uint64,
	taskID string,
	ownerID string,
	expectedVersion uint64,
	allowedStatuses []types.EvaluationStatue,
	extraCondition string,
	extraArgs []any,
	updates map[string]any,
	stateMatches func(*types.EvaluationTaskEntity) bool,
	action string,
) (*types.EvaluationTaskEntity, error) {
	var updated types.EvaluationTaskEntity
	query := r.db.WithContext(ctx).
		Model(&updated).
		Clauses(clause.Returning{}).
		Where("tenant_id = ? AND id = ? AND owner_id = ? AND version = ?", tenantID, taskID, ownerID, expectedVersion).
		Where("status IN ?", allowedStatuses)
	if extraCondition != "" {
		query = query.Where(extraCondition, extraArgs...)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("%s evaluation task %s: %w", action, taskID, result.Error)
	}
	if result.RowsAffected > 1 {
		return nil, fmt.Errorf(
			"%s evaluation task %s: invariant violation: updated %d rows",
			action,
			taskID,
			result.RowsAffected,
		)
	}
	if result.RowsAffected == 0 {
		return nil, r.classifyEvaluationTaskMutationConflict(
			ctx,
			tenantID,
			taskID,
			ownerID,
			expectedVersion,
			stateMatches,
			action,
		)
	}
	normalizeEvaluationTaskTimes(&updated)
	return &updated, nil
}

func (r *evaluationTaskRepository) classifyEvaluationTaskMutationConflict(
	ctx context.Context,
	tenantID uint64,
	taskID string,
	ownerID string,
	expectedVersion uint64,
	stateMatches func(*types.EvaluationTaskEntity) bool,
	action string,
) error {
	task, err := r.GetTask(ctx, tenantID, taskID)
	if err != nil {
		return fmt.Errorf("%s evaluation task %s: %w", action, taskID, err)
	}
	if task.OwnerID != ownerID {
		return fmt.Errorf("%s evaluation task %s: %w", action, taskID, ErrEvaluationTaskOwnerConflict)
	}
	if stateMatches != nil && !stateMatches(task) {
		return fmt.Errorf("%s evaluation task %s: %w", action, taskID, ErrEvaluationTaskStateConflict)
	}
	if task.Version != expectedVersion {
		return fmt.Errorf("%s evaluation task %s: expected version %d, current version %d: %w",
			action, taskID, expectedVersion, task.Version, ErrEvaluationTaskVersionConflict)
	}
	return fmt.Errorf(
		"%s evaluation task %s: concurrent state changed: %w",
		action,
		taskID,
		ErrEvaluationTaskStateConflict,
	)
}

func validateEvaluationTaskMutationIdentity(tenantID uint64, taskID, ownerID string, expectedVersion uint64) error {
	if tenantID == 0 || taskID == "" || ownerID == "" || expectedVersion == 0 {
		return errors.New("tenant_id, task_id, owner_id, and expected_version are required")
	}
	return nil
}

func validateEvaluationTaskJSONObject(value types.JSON, allowEmpty bool) error {
	if len(value) == 0 {
		if allowEmpty {
			return nil
		}
		return errors.New("JSON object is required")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return errors.New("must be a valid JSON object")
	}
	return nil
}

func validateEvaluationTaskStringArray(value types.JSON) error {
	var values []string
	if err := json.Unmarshal(value, &values); err != nil || values == nil {
		return errors.New("must be a valid JSON string array")
	}
	return nil
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
