package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEvaluationTaskRepositoryPostgresContract(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	tx := db.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })

	schema := fmt.Sprintf("m2a_repository_%d", time.Now().UnixNano())
	require.NoError(t, tx.Exec("CREATE SCHEMA "+schema).Error)
	require.NoError(t, tx.Exec("SET LOCAL search_path = "+schema+", pg_catalog").Error)

	migrationPath := filepath.Join("..", "..", "..", "migrations", "versioned", "000090_evaluation_tasks.up.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	require.NoError(t, tx.Exec(string(migrationSQL)).Error)
	cancelMigrationPath := filepath.Join(
		"..", "..", "..", "migrations", "versioned", "000091_evaluation_task_cancellation.up.sql",
	)
	cancelMigrationSQL, err := os.ReadFile(cancelMigrationPath)
	require.NoError(t, err)
	require.NoError(t, tx.Exec(string(cancelMigrationSQL)).Error)

	now := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	leaseExpiresAt := now.Add(time.Minute)
	task := &types.EvaluationTaskEntity{
		ID:                       "postgres-evaluation-task",
		TenantID:                 7,
		DatasetID:                "default",
		StartTime:                now,
		Params:                   types.JSON(`{"chat_model_id":"chat-1"}`),
		TemporaryKnowledgeBaseID: "kb-postgres",
		OwnerID:                  "owner-postgres",
		LeaseExpiresAt:           &leaseExpiresAt,
		HeartbeatAt:              now,
	}
	repo := NewEvaluationTaskRepository(tx)

	require.NoError(t, repo.CreateTask(context.Background(), task.TenantID, task))
	got, err := repo.GetTask(context.Background(), task.TenantID, task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, task.TenantID, got.TenantID)
	assert.Equal(t, task.Status, got.Status)
	assert.Equal(t, time.UTC, got.LeaseExpiresAt.Location())
	assert.JSONEq(t, task.Params.ToString(), got.Params.ToString())

	err = repo.CreateTask(context.Background(), task.TenantID, task)
	require.ErrorIs(t, err, ErrEvaluationTaskAlreadyExists)
}
