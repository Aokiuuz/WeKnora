package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEvaluationMethodSignaturesRemainCompatible(t *testing.T) {
	client := NewClient("http://example.test")
	var start func(context.Context, *EvaluationRequest) (*EvaluationTask, error) = client.StartEvaluation
	var get func(context.Context, string) (*EvaluationResult, error) = client.GetEvaluationResult
	if start == nil || get == nil {
		t.Fatal("evaluation methods must remain available")
	}
}

func TestStartEvaluationUsesServerRequestAndNestedTaskContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/evaluation" {
			t.Errorf("path = %s, want /api/v1/evaluation", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		want := map[string]any{
			"dataset_id":        "default",
			"knowledge_base_id": "kb-1",
			"chat_id":           "chat-1",
			"rerank_id":         "rerank-1",
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("request body = %#v, want %#v", body, want)
		}

		writeEvaluationResponse(t, w, map[string]any{
			"task": evaluationTaskFixture(1),
			"params": map[string]any{
				"chat_model_id":   "chat-1",
				"rerank_model_id": "rerank-1",
			},
		})
	}))
	defer srv.Close()

	request := &EvaluationRequest{
		DatasetID:       "default",
		KnowledgeBaseID: "kb-1",
		ChatModelID:     "chat-1",
		RerankModelID:   "rerank-1",
	}

	client := NewClient(srv.URL)
	task, err := client.StartEvaluation(context.Background(), request)
	if err != nil {
		t.Fatalf("StartEvaluation() error = %v", err)
	}
	if task.Status != EvaluationStatusRunning {
		t.Fatalf("task.Status = %d, want %d", task.Status, EvaluationStatusRunning)
	}
	if task.ID != "evaluation-1" {
		t.Fatalf("task.ID = %q, want evaluation-1", task.ID)
	}
}

func TestGetEvaluationResultParsesNestedDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/evaluation" {
			t.Errorf("path = %s, want /api/v1/evaluation", r.URL.Path)
		}
		if got := r.URL.Query().Get("task_id"); got != "evaluation-1" {
			t.Errorf("task_id = %q, want evaluation-1", got)
		}

		writeEvaluationResponse(t, w, map[string]any{
			"task": evaluationTaskFixture(2),
			"params": map[string]any{
				"chat_model_id": "chat-1",
			},
			"metric": map[string]any{
				"retrieval_metrics":  map[string]any{"precision": 0.5},
				"generation_metrics": map[string]any{"bleu1": 0.25},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	result, err := client.GetEvaluationResult(context.Background(), "evaluation-1")
	if err != nil {
		t.Fatalf("GetEvaluationResult() error = %v", err)
	}

	if result.Task == nil {
		t.Fatal("EvaluationResult.Task must contain the nested task")
	}
	if got := result.Task.ID; got != "evaluation-1" {
		t.Fatalf("result.Task.ID = %q, want evaluation-1", got)
	}
	if result.Task.Status != EvaluationStatusSuccess {
		t.Fatalf("result.Task.Status = %d, want %d", result.Task.Status, EvaluationStatusSuccess)
	}
	if len(result.Params) == 0 {
		t.Fatal("EvaluationResult.Params must preserve the nested response")
	}
	if result.Metric == nil {
		t.Fatal("EvaluationResult.Metric must preserve the nested response")
	}
}

func TestGetEvaluationResultRejectsStringStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		task := evaluationTaskFixture(1)
		task["status"] = "running"
		writeEvaluationResponse(t, w, map[string]any{
			"task":   task,
			"params": map[string]any{},
		})
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).GetEvaluationResult(context.Background(), "evaluation-1")
	if err == nil {
		t.Fatal("GetEvaluationResult() must reject a string status")
	}
}

func TestGetEvaluationResultRejectsMissingNestedTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEvaluationResponse(t, w, map[string]any{
			"params": map[string]any{},
		})
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).GetEvaluationResult(context.Background(), "evaluation-1")
	if err == nil {
		t.Fatal("GetEvaluationResult() must reject a response without data.task")
	}
}

func TestStartEvaluationRejectsDeprecatedEmbeddingModelField(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).StartEvaluation(context.Background(), &EvaluationRequest{
		DatasetID:        "default",
		EmbeddingModelID: "embedding-1",
	})
	if !errors.Is(err, ErrEvaluationEmbeddingModelUnsupported) {
		t.Fatalf("StartEvaluation() error = %v, want explicit EmbeddingModelID error", err)
	}
	if called {
		t.Fatal("deprecated embedding field must fail before sending an HTTP request")
	}
}

func evaluationTaskFixture(status int) map[string]any {
	return map[string]any{
		"id":         "evaluation-1",
		"tenant_id":  7,
		"dataset_id": "default",
		"start_time": "2026-08-28T08:00:00Z",
		"status":     status,
		"total":      4,
		"finished":   2,
	}
}

func writeEvaluationResponse(t *testing.T, w http.ResponseWriter, data map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
