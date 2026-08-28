package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newListTask(
	tenantID uint64,
	id string,
	startTime time.Time,
	status types.EvaluationStatue,
) *types.EvaluationTaskEntity {
	leaseExpiresAt := startTime.Add(time.Hour)
	return &types.EvaluationTaskEntity{
		ID:                       id,
		TenantID:                 tenantID,
		DatasetID:                "default",
		Status:                   status,
		StartTime:                startTime,
		TemporaryKnowledgeBaseID: "kb-" + id,
		OwnerID:                  "owner-" + id,
		LeaseExpiresAt:           &leaseExpiresAt,
		HeartbeatAt:              startTime,
		CreatedAt:                startTime,
		UpdatedAt:                startTime,
	}
}

func TestEvaluationTaskRepositoryListTasksKeysetHasNoDuplicateOrGap(t *testing.T) {
	db := setupEvaluationTaskRepositoryTestDB(t)
	repo := NewEvaluationTaskRepository(db)
	ctx := context.Background()

	// Five tasks share one start_time, so the id tiebreaker drives pagination.
	sharedStart := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		task := newListTask(71, fmt.Sprintf("shared-%d", i), sharedStart, types.EvaluationStatueSuccess)
		endTime := sharedStart.Add(time.Minute)
		task.EndTime = &endTime
		task.LeaseExpiresAt = nil
		require.NoError(t, db.Create(task).Error)
	}
	older := newListTask(71, "older", sharedStart.Add(-time.Hour), types.EvaluationStatueFailed)
	olderEnd := older.StartTime.Add(time.Minute)
	older.EndTime = &olderEnd
	older.LeaseExpiresAt = nil
	require.NoError(t, db.Create(older).Error)

	seen := make([]string, 0, 6)
	var startBefore *time.Time
	idBefore := ""
	for page := 0; page < 4; page++ {
		query := types.EvaluationTaskListQuery{Limit: 2, StartBefore: startBefore, IDBefore: idBefore}
		tasks, err := repo.ListTasks(ctx, 71, query)
		require.NoError(t, err)
		if len(tasks) == 0 {
			break
		}
		for _, task := range tasks {
			seen = append(seen, task.ID)
		}
		last := tasks[len(tasks)-1]
		boundary := last.StartTime.UTC()
		startBefore = &boundary
		idBefore = last.ID
	}
	assert.ElementsMatch(t,
		[]string{"shared-0", "shared-1", "shared-2", "shared-3", "shared-4", "older"},
		seen,
		"keyset walk must cover every task exactly once",
	)
	assert.Equal(t,
		[]string{"shared-4", "shared-3", "shared-2", "shared-1", "shared-0", "older"},
		seen,
		"ordering must be (start_time DESC, id DESC)",
	)
}

func TestEvaluationTaskRepositoryListTasksFiltersStatusTenantAndSoftDelete(t *testing.T) {
	db := setupEvaluationTaskRepositoryTestDB(t)
	repo := NewEvaluationTaskRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)

	running := newListTask(72, "running", now, types.EvaluationStatueRunning)
	require.NoError(t, db.Create(running).Error)
	succeeded := newListTask(72, "succeeded", now.Add(-time.Minute), types.EvaluationStatueSuccess)
	succeededEnd := succeeded.StartTime.Add(time.Minute)
	succeeded.EndTime = &succeededEnd
	succeeded.LeaseExpiresAt = nil
	require.NoError(t, db.Create(succeeded).Error)
	otherTenant := newListTask(73, "other-tenant", now, types.EvaluationStatueRunning)
	require.NoError(t, db.Create(otherTenant).Error)
	deleted := newListTask(72, "deleted", now.Add(-2*time.Minute), types.EvaluationStatueSuccess)
	deletedEnd := deleted.StartTime.Add(time.Minute)
	deleted.EndTime = &deletedEnd
	deleted.LeaseExpiresAt = nil
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Exec("UPDATE evaluation_tasks SET deleted_at = ? WHERE id = ?", now, deleted.ID).Error)

	status := types.EvaluationStatueRunning
	tasks, err := repo.ListTasks(ctx, 72, types.EvaluationTaskListQuery{Status: &status, Limit: 10})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, running.ID, tasks[0].ID)

	tasks, err = repo.ListTasks(ctx, 72, types.EvaluationTaskListQuery{Limit: 10})
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, []string{running.ID, succeeded.ID}, []string{tasks[0].ID, tasks[1].ID})
}

func TestEvaluationTaskRepositoryListTasksValidatesInput(t *testing.T) {
	db := setupEvaluationTaskRepositoryTestDB(t)
	repo := NewEvaluationTaskRepository(db)
	ctx := context.Background()

	_, err := repo.ListTasks(ctx, 0, types.EvaluationTaskListQuery{Limit: 1})
	require.Error(t, err)
	_, err = repo.ListTasks(ctx, 74, types.EvaluationTaskListQuery{Limit: 0})
	require.Error(t, err)
	boundary := time.Now().UTC()
	_, err = repo.ListTasks(ctx, 74, types.EvaluationTaskListQuery{Limit: 1, StartBefore: &boundary})
	require.Error(t, err, "boundary requires both start_time and id")
	_, err = repo.ListTasks(ctx, 74, types.EvaluationTaskListQuery{Limit: 1, IDBefore: "x"})
	require.Error(t, err, "boundary requires both start_time and id")
	unknown := types.EvaluationStatue(99)
	_, err = repo.ListTasks(ctx, 74, types.EvaluationTaskListQuery{Limit: 1, Status: &unknown})
	require.Error(t, err, "unknown status must be rejected")
}
