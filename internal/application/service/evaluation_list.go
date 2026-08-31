package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	evaluationDefaultListPageSize = 20
	evaluationMaxListPageSize     = 100
	evaluationListCursorVersion   = 1
)

// ErrEvaluationTaskListInvalidCursor rejects malformed, outdated, or
// cross-filter cursors with a client-visible 400.
var ErrEvaluationTaskListInvalidCursor = errors.New("evaluation task list cursor is invalid")

type evaluationTaskListCursor struct {
	Version    int       `json:"v"`
	StartTime  time.Time `json:"start_time"`
	ID         string    `json:"id"`
	FilterHash string    `json:"filter"`
}

func evaluationTaskListFilterHash(status *types.EvaluationStatue) string {
	canonical, err := json.Marshal(struct {
		Status *types.EvaluationStatue `json:"status"`
	}{Status: status})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func encodeEvaluationTaskListCursor(task *types.EvaluationTaskEntity, status *types.EvaluationStatue) (string, error) {
	if task == nil {
		return "", nil
	}
	payload, err := json.Marshal(evaluationTaskListCursor{
		Version:    evaluationListCursorVersion,
		StartTime:  task.StartTime.UTC(),
		ID:         task.ID,
		FilterHash: evaluationTaskListFilterHash(status),
	})
	if err != nil {
		return "", fmt.Errorf("encode evaluation task list cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeEvaluationTaskListCursor(
	encoded string,
	status *types.EvaluationStatue,
) (*evaluationTaskListCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed encoding", ErrEvaluationTaskListInvalidCursor)
	}
	var cursor evaluationTaskListCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return nil, fmt.Errorf("%w: malformed payload", ErrEvaluationTaskListInvalidCursor)
	}
	if cursor.Version != evaluationListCursorVersion {
		return nil, fmt.Errorf("%w: unsupported version", ErrEvaluationTaskListInvalidCursor)
	}
	if cursor.StartTime.IsZero() || cursor.ID == "" {
		return nil, fmt.Errorf("%w: missing keyset boundary", ErrEvaluationTaskListInvalidCursor)
	}
	if cursor.FilterHash != evaluationTaskListFilterHash(status) {
		return nil, fmt.Errorf("%w: filter changed", ErrEvaluationTaskListInvalidCursor)
	}
	return &cursor, nil
}

// ListEvaluations returns one keyset page of the current tenant's evaluation
// tasks ordered by (start_time DESC, id DESC). Soft-deleted tasks are hidden.
func (e *EvaluationService) ListEvaluations(
	ctx context.Context,
	input types.EvaluationTaskListInput,
) (*types.EvaluationTaskListPage, error) {
	tenantID := types.MustTenantIDFromContext(ctx)

	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = evaluationDefaultListPageSize
	}
	if pageSize > evaluationMaxListPageSize {
		pageSize = evaluationMaxListPageSize
	}
	if input.Status != nil && !isKnownEvaluationStatus(*input.Status) {
		return nil, fmt.Errorf(
			"%w: unsupported status filter %d",
			ErrEvaluationTaskListInvalidCursor,
			*input.Status,
		)
	}

	query := types.EvaluationTaskListQuery{
		Status: input.Status,
		Limit:  pageSize + 1,
	}
	if input.Cursor != "" {
		cursor, err := decodeEvaluationTaskListCursor(input.Cursor, input.Status)
		if err != nil {
			return nil, err
		}
		startBefore := cursor.StartTime.UTC()
		query.StartBefore = &startBefore
		query.IDBefore = cursor.ID
	}

	tasks := make([]*types.EvaluationTaskEntity, 0, pageSize+1)
	scanQuery := query
	for len(tasks) <= pageSize {
		batch, err := e.evaluationTaskRepository.ListTasks(ctx, tenantID, scanQuery)
		if err != nil {
			return nil, err
		}
		for _, task := range batch {
			if AuthorizeEvaluationTaskForAPIKey(ctx, task) == nil {
				tasks = append(tasks, task)
				if len(tasks) > pageSize {
					break
				}
			}
		}
		if len(tasks) > pageSize || len(batch) < scanQuery.Limit || len(batch) == 0 {
			break
		}
		lastScanned := batch[len(batch)-1]
		startBefore := lastScanned.StartTime.UTC()
		scanQuery.StartBefore = &startBefore
		scanQuery.IDBefore = lastScanned.ID
	}

	page := &types.EvaluationTaskListPage{Items: tasks}
	if len(tasks) > pageSize {
		page.Items = tasks[:pageSize]
		nextCursor, err := encodeEvaluationTaskListCursor(tasks[pageSize-1], input.Status)
		if err != nil {
			return nil, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}
