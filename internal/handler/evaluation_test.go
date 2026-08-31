package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubEvaluationService struct {
	lastOptions *types.EvaluationOptions
	err         error
}

func (s *stubEvaluationService) Evaluation(
	context.Context, string, string, string, string,
) (*types.EvaluationDetail, error) {
	return nil, fmt.Errorf("legacy entry not used by handler tests")
}

func (s *stubEvaluationService) EvaluationWithOptions(
	_ context.Context, options *types.EvaluationOptions,
) (*types.EvaluationDetail, error) {
	s.lastOptions = options
	if s.err != nil {
		return nil, s.err
	}
	return &types.EvaluationDetail{
		Task: &types.EvaluationTask{ID: "evaluation-stub", Status: types.EvaluationStatuePending},
		Params: &types.ChatManage{
			PipelineRequest: types.PipelineRequest{ChatModelID: "chat-1"},
		},
	}, nil
}

func (s *stubEvaluationService) EvaluationResult(
	context.Context, string,
) (*types.EvaluationDetail, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubEvaluationService) CancelEvaluation(
	context.Context, string,
) (*types.EvaluationDetail, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubEvaluationService) ListEvaluations(
	context.Context, types.EvaluationTaskListInput,
) (*types.EvaluationTaskListPage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubEvaluationService) ReplaceEvaluationTaskLabels(
	context.Context, string, []string,
) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *stubEvaluationService) DeleteEvaluation(context.Context, string) error {
	return fmt.Errorf("not implemented")
}

func setupEvaluationHandlerRouter(svc *stubEvaluationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(string(types.TenantIDContextKey), uint64(7))
		c.Next()
	})
	h := NewEvaluationHandler(svc)
	engine.POST("/api/v1/evaluation", h.Evaluation)
	return engine
}

func TestEvaluationHandlerForwardsSeedAndDatasetVersion(t *testing.T) {
	svc := &stubEvaluationService{}
	engine := setupEvaluationHandlerRouter(svc)

	body := strings.NewReader(`{
		"dataset_id": "default",
		"chat_id": "chat-1",
		"dataset_version_id": "version-9",
		"seed": 0,
		"configuration": {"generation": {"temperature": 0.1}}
	}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/evaluation", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, svc.lastOptions)
	assert.Equal(t, "version-9", svc.lastOptions.DatasetVersionID)
	require.NotNil(t, svc.lastOptions.Seed, "explicit seed=0 must survive as a pointer")
	assert.Equal(t, 0, *svc.lastOptions.Seed)
	require.NotNil(t, svc.lastOptions.Configuration)
	require.NotNil(t, svc.lastOptions.Configuration.Generation)
	require.NotNil(t, svc.lastOptions.Configuration.Generation.Temperature)
	assert.InDelta(t, 0.1, *svc.lastOptions.Configuration.Generation.Temperature, 1e-9)
}

func TestEvaluationHandlerOmitsUnprovidedSeed(t *testing.T) {
	svc := &stubEvaluationService{}
	engine := setupEvaluationHandlerRouter(svc)

	body := strings.NewReader(`{"dataset_id": "default", "chat_id": "chat-1"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/evaluation", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.NotNil(t, svc.lastOptions)
	assert.Nil(t, svc.lastOptions.Seed, "an absent seed must stay nil, never a silent zero")
}

func TestEvaluationHandlerMapsSeedUnsupportedTo422(t *testing.T) {
	svc := &stubEvaluationService{
		err: fmt.Errorf("evaluation seed 0: chat model claude: %w", service.ErrEvaluationSeedUnsupported),
	}
	engine := setupEvaluationHandlerRouter(svc)

	body := strings.NewReader(`{"dataset_id": "default", "chat_id": "claude", "seed": 0}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/evaluation", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.Contains(t, response.Body.String(), "seed")
}
