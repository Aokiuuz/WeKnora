package handler

import (
	stderrors "errors"
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// EvaluationHandler handles evaluation related HTTP requests
type EvaluationHandler struct {
	evaluationService interfaces.EvaluationService // Service for evaluation operations
}

// NewEvaluationHandler creates a new EvaluationHandler instance
func NewEvaluationHandler(evaluationService interfaces.EvaluationService) *EvaluationHandler {
	return &EvaluationHandler{evaluationService: evaluationService}
}

// EvaluationRequest contains parameters for evaluation request
type EvaluationRequest struct {
	DatasetID       string `json:"dataset_id"`        // ID of dataset to evaluate
	KnowledgeBaseID string `json:"knowledge_base_id"` // ID of knowledge base to use
	ChatModelID     string `json:"chat_id"`           // ID of chat model to use
	RerankModelID   string `json:"rerank_id"`         // ID of rerank model to use
}

// Evaluation godoc
// @Summary      执行评估
// @Description  对知识库进行评估测试
// @Tags         评估
// @Accept       json
// @Produce      json
// @Param        request  body      EvaluationRequest  true  "评估请求参数"
// @Success      200      {object}  map[string]interface{}  "评估任务"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /evaluation/ [post]
func (e *EvaluationHandler) Evaluation(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start processing evaluation request")

	var request EvaluationRequest
	if err := c.ShouldBind(&request); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	tenantID, exists := c.Get(string(types.TenantIDContextKey))
	if !exists {
		logger.Error(ctx, "Failed to get tenant ID")
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	logger.Infof(ctx, "Executing evaluation, tenant: %v, dataset: %s, knowledge_base: %s, chat: %s, rerank: %s",
		tenantID,
		secutils.SanitizeForLog(request.DatasetID),
		secutils.SanitizeForLog(request.KnowledgeBaseID),
		secutils.SanitizeForLog(request.ChatModelID),
		secutils.SanitizeForLog(request.RerankModelID),
	)

	task, err := e.evaluationService.Evaluation(ctx,
		secutils.SanitizeForLog(request.DatasetID),
		secutils.SanitizeForLog(request.KnowledgeBaseID),
		secutils.SanitizeForLog(request.ChatModelID),
		secutils.SanitizeForLog(request.RerankModelID),
	)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Evaluation task created successfully")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    task,
	})
}

// GetEvaluationRequest contains parameters for getting evaluation result
type GetEvaluationRequest struct {
	TaskID string `form:"task_id" binding:"required"` // ID of evaluation task
}

// ListEvaluationTasks godoc
// @Summary      列出评估任务
// @Description  按 (start_time DESC, id DESC) keyset 分页列出当前租户的评估任务
// @Tags         评估
// @Accept       json
// @Produce      json
// @Param        status     query     int     false  "数值状态筛选"
// @Param        page_size  query     int     false  "每页条数（默认 20，最大 100）"
// @Param        cursor     query     string  false  "上一页返回的 next_cursor"
// @Success      200        {object}  map[string]interface{}  "任务页"
// @Failure      400        {object}  errors.AppError         "非法游标或筛选"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /evaluation/tasks [get]
func (e *EvaluationHandler) ListEvaluationTasks(c *gin.Context) {
	ctx := c.Request.Context()

	var input types.EvaluationTaskListInput
	if raw := c.Query("status"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			_ = c.Error(errors.NewBadRequestError("status must be a numeric evaluation status"))
			return
		}
		status := types.EvaluationStatue(value)
		input.Status = &status
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			_ = c.Error(errors.NewBadRequestError("page_size must be numeric"))
			return
		}
		input.PageSize = value
	}
	input.Cursor = c.Query("cursor")

	page, err := e.evaluationService.ListEvaluations(ctx, input)
	if err != nil {
		if stderrors.Is(err, service.ErrEvaluationTaskListInvalidCursor) {
			_ = c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		_ = c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	items := make([]*types.EvaluationTask, 0, len(page.Items))
	for _, entity := range page.Items {
		task, err := service.EvaluationTaskEntityToAPITask(entity)
		if err != nil {
			logger.ErrorWithFields(ctx, err, nil)
			_ = c.Error(errors.NewInternalServerError(err.Error()))
			return
		}
		items = append(items, task)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":       items,
			"next_cursor": page.NextCursor,
		},
	})
}

// CancelEvaluation godoc
// @Summary      取消评估任务
// @Description  持久化取消请求；运行实例处理取消并完成资源清理后任务进入 Canceled
// @Tags         评估
// @Accept       json
// @Produce      json
// @Param        task_id  path      string  true  "评估任务ID"
// @Success      200      {object}  map[string]interface{}  "当前任务状态"
// @Failure      404      {object}  errors.AppError         "任务不存在或属于其他租户"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /evaluation/{task_id}/cancel [post]
func (e *EvaluationHandler) CancelEvaluation(c *gin.Context) {
	ctx := c.Request.Context()

	taskID := c.Param("task_id")
	if taskID == "" {
		_ = c.Error(errors.NewBadRequestError("task_id is required"))
		return
	}
	logger.Infof(ctx, "Processing evaluation cancel request, task ID: %s", secutils.SanitizeForLog(taskID))

	result, err := e.evaluationService.CancelEvaluation(ctx, taskID)
	if err != nil {
		if stderrors.Is(err, interfaces.ErrEvaluationTaskNotFound) {
			_ = c.Error(errors.NewNotFoundError("Evaluation task not found"))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		_ = c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetEvaluationResult godoc
// @Summary      获取评估结果
// @Description  根据任务ID获取评估结果
// @Tags         评估
// @Accept       json
// @Produce      json
// @Param        task_id  query     string  true  "评估任务ID"
// @Success      200      {object}  map[string]interface{}  "评估结果"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /evaluation/ [get]
func (e *EvaluationHandler) GetEvaluationResult(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start retrieving evaluation result")

	var request GetEvaluationRequest
	if err := c.ShouldBind(&request); err != nil {
		logger.Error(ctx, "Failed to parse request parameters", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	result, err := e.evaluationService.EvaluationResult(ctx, secutils.SanitizeForLog(request.TaskID))
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Info(ctx, "Retrieved evaluation result successfully")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
