package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const evaluationTaskTestDDL = `
CREATE TABLE evaluation_tasks (
    id                     TEXT PRIMARY KEY,
    tenant_id              INTEGER NOT NULL,
    dataset_id             TEXT NOT NULL,
    status                 INTEGER NOT NULL,
    start_time             DATETIME NOT NULL,
    end_time               DATETIME,
    total                  INTEGER NOT NULL DEFAULT 0,
    finished               INTEGER NOT NULL DEFAULT 0,
    err_msg                TEXT NOT NULL DEFAULT '',
    cleanup_errors         TEXT NOT NULL DEFAULT '[]',
    params                 TEXT NOT NULL DEFAULT '{}',
    metric                 TEXT,
    temporary_kb_id        TEXT NOT NULL,
    temporary_knowledge_id TEXT,
    owner_id               TEXT NOT NULL,
    lease_expires_at       DATETIME NOT NULL,
    heartbeat_at           DATETIME NOT NULL,
    version                INTEGER NOT NULL DEFAULT 1,
    created_at             DATETIME NOT NULL,
    updated_at             DATETIME NOT NULL,
    deleted_at             DATETIME
);
`

func setupEvaluationTaskRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:evaluation-task-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(evaluationTaskTestDDL).Error)
	return db
}

func TestEvaluationTaskRepositoryPersistsTenantScopedSnapshot(t *testing.T) {
	db := setupEvaluationTaskRepositoryTestDB(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	leaseExpiresAt := createdAt.Add(time.Minute)
	task := &types.EvaluationTaskEntity{
		ID:                       "evaluation_7_1787875200000_a1b2c3d4_default",
		TenantID:                 7,
		DatasetID:                "default",
		Status:                   types.EvaluationStatuePending,
		StartTime:                createdAt,
		CleanupErrors:            types.JSON(`["cleanup warning"]`),
		Params:                   types.JSON(`{"chat_model_id":"chat-1"}`),
		Metric:                   types.JSON(`{"retrieval_metrics":{"precision":0.5}}`),
		TemporaryKnowledgeBaseID: "kb-evaluation",
		OwnerID:                  "a1b2c3d4-e5f6-47a8-9012-3456789abcde",
		LeaseExpiresAt:           leaseExpiresAt,
		HeartbeatAt:              createdAt,
		Version:                  1,
		CreatedAt:                createdAt,
		UpdatedAt:                createdAt,
	}

	repo := NewEvaluationTaskRepository(db)
	require.NoError(t, repo.CreateTask(ctx, task))

	// A fresh repository instance represents a service reconstructed after a
	// process restart. The database row remains the source of truth.
	restartedRepo := NewEvaluationTaskRepository(db)
	got, err := restartedRepo.GetTask(ctx, task.TenantID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, task.TenantID, got.TenantID)
	assert.Equal(t, task.DatasetID, got.DatasetID)
	assert.Equal(t, task.Status, got.Status)
	assert.Equal(t, task.StartTime, got.StartTime)
	assert.Equal(t, task.TemporaryKnowledgeBaseID, got.TemporaryKnowledgeBaseID)
	assert.Equal(t, task.OwnerID, got.OwnerID)
	assert.Equal(t, task.LeaseExpiresAt, got.LeaseExpiresAt)
	assert.JSONEq(t, task.CleanupErrors.ToString(), got.CleanupErrors.ToString())
	assert.JSONEq(t, task.Params.ToString(), got.Params.ToString())
	assert.JSONEq(t, task.Metric.ToString(), got.Metric.ToString())

	_, err = restartedRepo.GetTask(ctx, 8, task.ID)
	require.ErrorIs(t, err, ErrEvaluationTaskNotFound)
}

func TestEvaluationTaskRepositoryCreateDefaultsAndSoftDelete(t *testing.T) {
	db := setupEvaluationTaskRepositoryTestDB(t)
	ctx := context.Background()
	task := &types.EvaluationTaskEntity{
		ID:                       "evaluation_9_1787875200000_b1c2d3e4_default",
		TenantID:                 9,
		DatasetID:                "default",
		TemporaryKnowledgeBaseID: "kb-evaluation-defaults",
		OwnerID:                  "b1c2d3e4-f5a6-47b8-9012-3456789abcde",
		LeaseExpiresAt:           time.Now().UTC().Add(time.Minute),
	}
	repo := NewEvaluationTaskRepository(db)

	require.NoError(t, repo.CreateTask(ctx, task))
	got, err := repo.GetTask(ctx, task.TenantID, task.ID)
	require.NoError(t, err)
	assert.False(t, got.StartTime.IsZero())
	assert.False(t, got.HeartbeatAt.IsZero())
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
	assert.Equal(t, uint64(1), got.Version)
	assert.JSONEq(t, `[]`, got.CleanupErrors.ToString())
	assert.JSONEq(t, `{}`, got.Params.ToString())

	deleteResult := db.Delete(
		&types.EvaluationTaskEntity{}, "tenant_id = ? AND id = ?", task.TenantID, task.ID,
	)
	require.NoError(t, deleteResult.Error)
	_, err = repo.GetTask(ctx, task.TenantID, task.ID)
	require.ErrorIs(t, err, ErrEvaluationTaskNotFound)
}

func TestEvaluationTaskRepositoryRejectsIncompleteTask(t *testing.T) {
	db := setupEvaluationTaskRepositoryTestDB(t)
	repo := NewEvaluationTaskRepository(db)
	ctx := context.Background()
	leaseExpiresAt := time.Now().UTC().Add(time.Minute)

	tests := []struct {
		name string
		task *types.EvaluationTaskEntity
	}{
		{name: "nil task"},
		{name: "missing identity", task: &types.EvaluationTaskEntity{}},
		{name: "missing owner", task: &types.EvaluationTaskEntity{
			ID: "task", TenantID: 1, DatasetID: "default",
			TemporaryKnowledgeBaseID: "kb", LeaseExpiresAt: leaseExpiresAt,
		}},
		{name: "missing lease", task: &types.EvaluationTaskEntity{
			ID: "task", TenantID: 1, DatasetID: "default",
			TemporaryKnowledgeBaseID: "kb", OwnerID: "owner",
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, repo.CreateTask(ctx, tc.task))
		})
	}

	var count int64
	require.NoError(t, db.Model(&types.EvaluationTaskEntity{}).Count(&count).Error)
	assert.Zero(t, count)
}
