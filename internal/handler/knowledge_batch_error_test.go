package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type batchFailureKnowledgeService struct{ interfaces.KnowledgeService }

func (*batchFailureKnowledgeService) GetKnowledgeBatch(context.Context, uint64, []string) ([]*types.Knowledge, error) {
	return nil, errors.New("fixture database unavailable")
}

type batchFailureKBService struct {
	interfaces.KnowledgeBaseService
}

func (*batchFailureKBService) GetKnowledgeBaseByID(context.Context, string) (*types.KnowledgeBase, error) {
	return &types.KnowledgeBase{ID: "kb1", TenantID: 42}, nil
}

func TestKnowledgeBatchWithExplicitKBPropagatesServiceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(42))
		c.Next()
	})
	handler := &KnowledgeHandler{kgService: &batchFailureKnowledgeService{}, kbService: &batchFailureKBService{}}
	router.GET("/batch", handler.GetKnowledgeBatch)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/batch?ids=k1&kb_id=kb1", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("service failure must not return success: status=%d body=%s", w.Code, w.Body.String())
	}
}

type nilBatchAgentService struct{ interfaces.AgentShareService }

func (*nilBatchAgentService) GetSharedAgentForTenant(
	context.Context, uint64, types.TenantRole, string, ...uint64,
) (*types.CustomAgent, error) {
	return nil, nil
}

func TestKnowledgeBatchRejectsMissingSharedAgentWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(42))
		c.Set(types.UserIDContextKey.String(), "fixture-user")
		c.Next()
	})
	handler := &KnowledgeHandler{agentShareService: &nilBatchAgentService{}}
	router.GET("/batch", handler.GetKnowledgeBatch)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/batch?ids=k1&agent_id=missing", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing shared agent must be rejected: status=%d body=%s", w.Code, w.Body.String())
	}
}
