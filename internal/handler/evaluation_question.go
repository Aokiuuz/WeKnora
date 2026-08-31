package handler

import (
	"net/http"
	"strconv"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// EvaluationQuestionHandler serves the per-question pagination API.
type EvaluationQuestionHandler struct {
	questionResults interfaces.EvaluationQuestionResultRepository
}

// NewEvaluationQuestionHandler creates the per-question handler.
func NewEvaluationQuestionHandler(
	questionResults interfaces.EvaluationQuestionResultRepository,
) *EvaluationQuestionHandler {
	return &EvaluationQuestionHandler{questionResults: questionResults}
}

// ListQuestionResults godoc
// @Summary      逐题结果分页
// @Description  按 sample_index 升序分页读取一个评测任务的逐题结果
// @Tags         评估
// @Produce      json
// @Param        task_id    path      string  true   "任务 ID"
// @Param        page_size  query     int     false  "每页数量（默认 100，最大 500）"
// @Param        cursor     query     string  false  "keyset 游标"
// @Success      200        {object}  map[string]interface{}  "逐题分页"
// @Security     Bearer
// @Router       /evaluation/tasks/{task_id}/questions [get]
func (h *EvaluationQuestionHandler) ListQuestionResults(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, ok := evaluationHandlerTenantID(c)
	if !ok {
		return
	}
	taskID := c.Param("task_id")
	if taskID == "" {
		c.Error(apperrors.NewBadRequestError("task_id is required"))
		return
	}

	pageSize := types.EvaluationQuestionPageDefaultSize
	if raw := c.Query("page_size"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > types.EvaluationQuestionPageMaxSize {
			c.Error(apperrors.NewBadRequestError("page_size must be between 1 and 500"))
			return
		}
		pageSize = parsed
	}

	sampleIndexFrom := 0
	if raw := c.Query("cursor"); raw != "" {
		from, err := types.DecodeEvaluationQuestionCursor(taskID, raw)
		if err != nil {
			c.Error(apperrors.NewBadRequestError("invalid question results cursor"))
			return
		}
		sampleIndexFrom = from
	}

	// Read one extra row to decide whether another page exists.
	rows, err := h.questionResults.ListQuestionResults(ctx, tenantID, taskID, sampleIndexFrom, pageSize+1)
	if err != nil {
		c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	nextCursor := ""
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		nextCursor, err = types.EncodeEvaluationQuestionCursor(taskID, last.SampleIndex+1)
		if err != nil {
			c.Error(apperrors.NewInternalServerError(err.Error()))
			return
		}
		rows = rows[:pageSize]
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":       rows,
			"next_cursor": nextCursor,
		},
	})
}
