package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type evaluationTimeoutDatasetStub struct {
	interfaces.DatasetService
}

func (s *evaluationTimeoutDatasetStub) GetDatasetByID(
	context.Context,
	string,
) ([]*types.QAPair, error) {
	return []*types.QAPair{
		{
			QID:      0,
			Question: "question",
			PIDs:     []int{0},
			Passages: []string{"passage"},
			Answer:   "answer",
		},
	}, nil
}

type evaluationTimeoutKnowledgeStub struct {
	interfaces.KnowledgeService
	waitForDeleteDeadline bool
	deleteObserved        chan error
}

func (s *evaluationTimeoutKnowledgeStub) CreateKnowledgeFromPassageSync(
	context.Context,
	string,
	[]string,
	string,
) (*types.Knowledge, error) {
	return &types.Knowledge{ID: "evaluation-knowledge"}, nil
}

func (s *evaluationTimeoutKnowledgeStub) DeleteKnowledge(ctx context.Context, _ string) error {
	if s.waitForDeleteDeadline {
		<-ctx.Done()
		if s.deleteObserved != nil {
			s.deleteObserved <- ctx.Err()
		}
	}
	return nil
}

type evaluationTimeoutKnowledgeBaseStub struct {
	interfaces.KnowledgeBaseService
}

func (s *evaluationTimeoutKnowledgeBaseStub) DeleteKnowledgeBase(context.Context, string) error {
	return nil
}

type evaluationTimeoutSessionStub struct {
	interfaces.SessionService
	err           error
	observed      chan error
	safetyRelease <-chan struct{}
}

func (s *evaluationTimeoutSessionStub) KnowledgeQAByEvent(
	ctx context.Context,
	_ *types.ChatManage,
	_ []types.EventType,
) error {
	if s.err != nil {
		return s.err
	}
	select {
	case <-ctx.Done():
		if s.observed != nil {
			s.observed <- ctx.Err()
		}
		return ctx.Err()
	case <-s.safetyRelease:
		return nil
	}
}

func newEvaluationTimeoutDetail(storage *evaluationMemoryStorage) *types.EvaluationDetail {
	detail := &types.EvaluationDetail{
		Task: &types.EvaluationTask{
			ID:        "evaluation-task",
			DatasetID: "dataset",
			Status:    types.EvaluationStatuePending,
		},
		Params: &types.ChatManage{},
	}
	storage.register(detail)
	return detail
}

func TestEvaluationServiceMarksTaskTimedOut(t *testing.T) {
	storage := newEvaluationMemoryStorage()
	detail := newEvaluationTimeoutDetail(storage)
	observed := make(chan error, 1)
	safetyRelease := make(chan struct{})
	service := &EvaluationService{
		config: &config.Config{
			Evaluation: &config.EvaluationConfig{TaskTimeout: 20 * time.Millisecond},
		},
		dataset:                 &evaluationTimeoutDatasetStub{},
		knowledgeService:        &evaluationTimeoutKnowledgeStub{},
		knowledgeBaseService:    &evaluationTimeoutKnowledgeBaseStub{},
		sessionService:          &evaluationTimeoutSessionStub{observed: observed, safetyRelease: safetyRelease},
		evaluationMemoryStorage: storage,
	}

	result := make(chan error, 1)
	go func() {
		result <- service.runEvaluation(context.Background(), detail, "evaluation-kb")
	}()
	select {
	case runErr := <-result:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("runEvaluation() error = %v, want context.DeadlineExceeded", runErr)
		}
	case <-time.After(2 * time.Second):
		close(safetyRelease)
		select {
		case <-result:
		case <-time.After(2 * time.Second):
			t.Fatal("evaluation task did not stop after safety release")
		}
		t.Fatal("timed out waiting for evaluation task deadline")
	}

	select {
	case observedErr := <-observed:
		if !errors.Is(observedErr, context.DeadlineExceeded) {
			t.Fatalf("worker context error = %v, want context.DeadlineExceeded", observedErr)
		}
	default:
		t.Fatal("worker did not observe the evaluation task deadline")
	}
	terminal, err := storage.get(detail.Task.ID)
	if err != nil {
		t.Fatalf("storage.get() error = %v", err)
	}
	if terminal.Task.Status != types.EvaluationStatueTimedOut {
		t.Fatalf("terminal task status = %v, want TimedOut", terminal.Task.Status)
	}
	if terminal.Task.ErrMsg != context.DeadlineExceeded.Error() {
		t.Fatalf("terminal task error = %q, want %q", terminal.Task.ErrMsg, context.DeadlineExceeded)
	}
}

func TestEvaluationServiceKeepsBusinessErrorFailed(t *testing.T) {
	workerErr := errors.New("worker failed")
	storage := newEvaluationMemoryStorage()
	detail := newEvaluationTimeoutDetail(storage)
	service := &EvaluationService{
		config: &config.Config{
			Evaluation: &config.EvaluationConfig{TaskTimeout: time.Hour},
		},
		dataset:                 &evaluationTimeoutDatasetStub{},
		knowledgeService:        &evaluationTimeoutKnowledgeStub{},
		knowledgeBaseService:    &evaluationTimeoutKnowledgeBaseStub{},
		sessionService:          &evaluationTimeoutSessionStub{err: workerErr},
		evaluationMemoryStorage: storage,
	}

	runErr := service.runEvaluation(context.Background(), detail, "evaluation-kb")
	if !errors.Is(runErr, workerErr) {
		t.Fatalf("runEvaluation() error = %v, want errors.Is(error, workerErr)", runErr)
	}
	terminal, err := storage.get(detail.Task.ID)
	if err != nil {
		t.Fatalf("storage.get() error = %v", err)
	}
	if terminal.Task.Status != types.EvaluationStatueFailed {
		t.Fatalf("terminal task status = %v, want Failed", terminal.Task.Status)
	}
	if terminal.Task.ErrMsg != workerErr.Error() {
		t.Fatalf("terminal task error = %q, want %q", terminal.Task.ErrMsg, workerErr)
	}
}

func TestEvaluationServiceKeepsBusinessErrorWhenCleanupCrossesDeadline(t *testing.T) {
	workerErr := errors.New("worker failed before cleanup")
	storage := newEvaluationMemoryStorage()
	detail := newEvaluationTimeoutDetail(storage)
	deleteObserved := make(chan error, 1)
	service := &EvaluationService{
		config: &config.Config{
			Evaluation: &config.EvaluationConfig{TaskTimeout: 20 * time.Millisecond},
		},
		dataset: &evaluationTimeoutDatasetStub{},
		knowledgeService: &evaluationTimeoutKnowledgeStub{
			waitForDeleteDeadline: true,
			deleteObserved:        deleteObserved,
		},
		knowledgeBaseService:    &evaluationTimeoutKnowledgeBaseStub{},
		sessionService:          &evaluationTimeoutSessionStub{err: workerErr},
		evaluationMemoryStorage: storage,
	}

	runErr := service.runEvaluation(context.Background(), detail, "evaluation-kb")
	if !errors.Is(runErr, workerErr) {
		t.Fatalf("runEvaluation() error = %v, want errors.Is(error, workerErr)", runErr)
	}
	if observedErr := <-deleteObserved; !errors.Is(observedErr, context.DeadlineExceeded) {
		t.Fatalf("cleanup context error = %v, want context.DeadlineExceeded", observedErr)
	}
	terminal, err := storage.get(detail.Task.ID)
	if err != nil {
		t.Fatalf("storage.get() error = %v", err)
	}
	if terminal.Task.Status != types.EvaluationStatueFailed {
		t.Fatalf("terminal task status = %v, want Failed", terminal.Task.Status)
	}
	if terminal.Task.ErrMsg != workerErr.Error() {
		t.Fatalf("terminal task error = %q, want %q", terminal.Task.ErrMsg, workerErr)
	}
}

func TestEvaluationServiceKeepsDownstreamDeadlineFailureFailed(t *testing.T) {
	storage := newEvaluationMemoryStorage()
	detail := newEvaluationTimeoutDetail(storage)
	service := &EvaluationService{
		config: &config.Config{
			Evaluation: &config.EvaluationConfig{TaskTimeout: time.Hour},
		},
		dataset:                 &evaluationTimeoutDatasetStub{},
		knowledgeService:        &evaluationTimeoutKnowledgeStub{},
		knowledgeBaseService:    &evaluationTimeoutKnowledgeBaseStub{},
		sessionService:          &evaluationTimeoutSessionStub{err: context.DeadlineExceeded},
		evaluationMemoryStorage: storage,
	}

	runErr := service.runEvaluation(context.Background(), detail, "evaluation-kb")
	if !errors.Is(runErr, context.DeadlineExceeded) {
		t.Fatalf("runEvaluation() error = %v, want context.DeadlineExceeded", runErr)
	}
	terminal, err := storage.get(detail.Task.ID)
	if err != nil {
		t.Fatalf("storage.get() error = %v", err)
	}
	if terminal.Task.Status != types.EvaluationStatueFailed {
		t.Fatalf("terminal task status = %v, want Failed", terminal.Task.Status)
	}
}

func TestEvaluationServiceKeepsSuccessWhenOnlyCleanupCrossesDeadline(t *testing.T) {
	storage := newEvaluationMemoryStorage()
	detail := newEvaluationTimeoutDetail(storage)
	deleteObserved := make(chan error, 1)
	qaCompleted := make(chan struct{})
	close(qaCompleted)
	service := &EvaluationService{
		config: &config.Config{
			Evaluation: &config.EvaluationConfig{TaskTimeout: 20 * time.Millisecond},
		},
		dataset: &evaluationTimeoutDatasetStub{},
		knowledgeService: &evaluationTimeoutKnowledgeStub{
			waitForDeleteDeadline: true,
			deleteObserved:        deleteObserved,
		},
		knowledgeBaseService:    &evaluationTimeoutKnowledgeBaseStub{},
		sessionService:          &evaluationTimeoutSessionStub{safetyRelease: qaCompleted},
		evaluationMemoryStorage: storage,
	}

	if runErr := service.runEvaluation(context.Background(), detail, "evaluation-kb"); runErr != nil {
		t.Fatalf("runEvaluation() error = %v, want nil", runErr)
	}
	if observedErr := <-deleteObserved; !errors.Is(observedErr, context.DeadlineExceeded) {
		t.Fatalf("cleanup context error = %v, want context.DeadlineExceeded", observedErr)
	}
	terminal, err := storage.get(detail.Task.ID)
	if err != nil {
		t.Fatalf("storage.get() error = %v", err)
	}
	if terminal.Task.Status != types.EvaluationStatueSuccess {
		t.Fatalf("terminal task status = %v, want Success", terminal.Task.Status)
	}
}
