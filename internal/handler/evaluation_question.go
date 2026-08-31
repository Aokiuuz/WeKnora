package handler

import (
	"encoding/json"
	"errors"
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
	humanRatings    interfaces.EvaluationHumanRatingRepository
}

// NewEvaluationQuestionHandler creates the per-question handler.
func NewEvaluationQuestionHandler(
	questionResults interfaces.EvaluationQuestionResultRepository,
	humanRatings interfaces.EvaluationHumanRatingRepository,
) *EvaluationQuestionHandler {
	return &EvaluationQuestionHandler{questionResults: questionResults, humanRatings: humanRatings}
}

// ListHumanRatings returns newest-first immutable revisions for one question.
func (h *EvaluationQuestionHandler) ListHumanRatings(c *gin.Context) {
	tenantID, taskID, sampleIndex, ok := evaluationRatingIdentity(c)
	if !ok {
		return
	}
	items, err := h.humanRatings.ListHumanRatings(c.Request.Context(), tenantID, taskID, sampleIndex)
	if err != nil {
		if errors.Is(err, interfaces.ErrEvaluationTaskNotFound) {
			_ = c.Error(apperrors.NewNotFoundError("Evaluation question not found"))
			return
		}
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items}})
}

type appendHumanRatingRequest struct {
	RubricKey      string         `json:"rubric_key" binding:"required,max=64"`
	RubricVersion  string         `json:"rubric_version" binding:"required,max=32"`
	RubricSnapshot map[string]any `json:"rubric_snapshot" binding:"required"`
	Score          int            `json:"score" binding:"required,min=1,max=5"`
	Comment        string         `json:"comment" binding:"max=4000"`
}

// AppendHumanRating creates a new revision while retaining every prior judgment.
func (h *EvaluationQuestionHandler) AppendHumanRating(c *gin.Context) {
	tenantID, taskID, sampleIndex, ok := evaluationRatingIdentity(c)
	if !ok {
		return
	}
	var request appendHumanRatingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	rubric, err := json.Marshal(request.RubricSnapshot)
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError("rubric_snapshot must be a JSON object"))
		return
	}
	rating, err := h.humanRatings.AppendHumanRating(
		c.Request.Context(), tenantID, taskID, sampleIndex,
		c.GetString(types.UserIDContextKey.String()),
		types.EvaluationHumanRatingInput{
			RubricKey: request.RubricKey, RubricVersion: request.RubricVersion,
			RubricSnapshot: types.JSON(rubric), Score: request.Score, Comment: request.Comment,
		},
	)
	if err != nil {
		if errors.Is(err, interfaces.ErrEvaluationTaskNotFound) {
			_ = c.Error(apperrors.NewNotFoundError("Evaluation question not found"))
			return
		}
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": rating})
}

func evaluationRatingIdentity(c *gin.Context) (uint64, string, int, bool) {
	tenantID, ok := evaluationHandlerTenantID(c)
	if !ok {
		return 0, "", 0, false
	}
	taskID := c.Param("task_id")
	sampleIndex, err := strconv.Atoi(c.Param("sample_index"))
	if taskID == "" || err != nil || sampleIndex < 0 {
		_ = c.Error(apperrors.NewBadRequestError("task_id and non-negative sample_index are required"))
		return 0, "", 0, false
	}
	return tenantID, taskID, sampleIndex, true
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
		_ = c.Error(apperrors.NewBadRequestError("task_id is required"))
		return
	}

	pageSize := types.EvaluationQuestionPageDefaultSize
	if raw := c.Query("page_size"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > types.EvaluationQuestionPageMaxSize {
			_ = c.Error(apperrors.NewBadRequestError("page_size must be between 1 and 500"))
			return
		}
		pageSize = parsed
	}

	sampleIndexFrom := 0
	if raw := c.Query("cursor"); raw != "" {
		from, err := types.DecodeEvaluationQuestionCursor(taskID, raw)
		if err != nil {
			_ = c.Error(apperrors.NewBadRequestError("invalid question results cursor"))
			return
		}
		sampleIndexFrom = from
	}

	// Read one extra row to decide whether another page exists.
	rows, err := h.questionResults.ListQuestionResults(ctx, tenantID, taskID, sampleIndexFrom, pageSize+1)
	if err != nil {
		if errors.Is(err, interfaces.ErrEvaluationTaskNotFound) {
			_ = c.Error(apperrors.NewNotFoundError("Evaluation task not found"))
			return
		}
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}

	nextCursor := ""
	if len(rows) > pageSize {
		last := rows[pageSize-1]
		nextCursor, err = types.EncodeEvaluationQuestionCursor(taskID, last.SampleIndex+1)
		if err != nil {
			_ = c.Error(apperrors.NewInternalServerError(err.Error()))
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
