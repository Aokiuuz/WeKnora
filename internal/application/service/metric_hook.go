package service

import (
	"context"
	"strconv"
	"sync"

	"github.com/Tencent/WeKnora/internal/application/service/metric"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// MetricList stores and aggregates metric results
type MetricList struct {
	results []*types.MetricResult
}

// metricCalculators defines all metrics to be calculated
var metricCalculators = []struct {
	calc     interfaces.Metrics                 // Metric calculator implementation
	getField func(*types.MetricResult) *float64 // Field accessor for result
}{
	// Retrieval Metrics
	{metric.NewPrecisionMetric(), func(r *types.MetricResult) *float64 { return &r.RetrievalMetrics.Precision }},
	{metric.NewRecallMetric(), func(r *types.MetricResult) *float64 { return &r.RetrievalMetrics.Recall }},
	{metric.NewNDCGMetric(3), func(r *types.MetricResult) *float64 { return &r.RetrievalMetrics.NDCG3 }},
	{metric.NewNDCGMetric(10), func(r *types.MetricResult) *float64 { return &r.RetrievalMetrics.NDCG10 }},
	{metric.NewMRRMetric(), func(r *types.MetricResult) *float64 { return &r.RetrievalMetrics.MRR }},
	{metric.NewMAPMetric(), func(r *types.MetricResult) *float64 { return &r.RetrievalMetrics.MAP }},

	// Generation Metrics
	{metric.NewBLEUMetric(true, metric.BLEU1Gram), func(r *types.MetricResult) *float64 {
		return &r.GenerationMetrics.BLEU1
	}},
	{metric.NewBLEUMetric(true, metric.BLEU2Gram), func(r *types.MetricResult) *float64 {
		return &r.GenerationMetrics.BLEU2
	}},
	{metric.NewBLEUMetric(true, metric.BLEU4Gram), func(r *types.MetricResult) *float64 {
		return &r.GenerationMetrics.BLEU4
	}},
	{metric.NewRougeMetric(true, "rouge-1", "f"), func(r *types.MetricResult) *float64 {
		return &r.GenerationMetrics.ROUGE1
	}},
	{metric.NewRougeMetric(true, "rouge-2", "f"), func(r *types.MetricResult) *float64 {
		return &r.GenerationMetrics.ROUGE2
	}},
	{metric.NewRougeMetric(true, "rouge-l", "f"), func(r *types.MetricResult) *float64 {
		return &r.GenerationMetrics.ROUGEL
	}},
}

// Append calculates and stores metrics for given input
func (m *MetricList) Append(metricInput *types.MetricInput) {
	result := &types.MetricResult{}
	// Calculate all configured metrics
	for _, c := range metricCalculators {
		score := c.calc.Compute(metricInput)
		*c.getField(result) = score
	}
	logger.Infof(context.Background(), "metric: %v", result)
	m.results = append(m.results, result)
}

// Avg calculates average of all stored metric results
func (m *MetricList) Avg() *types.MetricResult {
	if len(m.results) == 0 {
		return &types.MetricResult{}
	}

	avgResult := &types.MetricResult{}
	count := float64(len(m.results))

	// Calculate average for each metric
	for _, config := range metricCalculators {
		sum := 0.0
		for _, r := range m.results {
			sum += *config.getField(r)
		}
		*config.getField(avgResult) = sum / count
	}
	return avgResult
}

// HookMetric tracks evaluation metrics for QA pairs
type HookMetric struct {
	qaPairMetricList []*qaPairMetric // Per-QA pair metrics
	metricResults    *MetricList     // Aggregated results
	knowledgeID      string          // Temporary evaluation knowledge source
	mu               *sync.RWMutex   // Thread safety
}

const unknownRetrievalID = -1

// qaPairMetric stores metrics for a single QA pair
type qaPairMetric struct {
	qaPair       *types.QAPair
	searchResult []*types.SearchResult
	rerankResult []*types.SearchResult
	chatResponse *types.ChatResponse
}

// NewHookMetric creates a new HookMetric with given capacity
func NewHookMetric(capacity int, knowledgeID string) *HookMetric {
	return &HookMetric{
		metricResults:    &MetricList{},
		qaPairMetricList: make([]*qaPairMetric, capacity),
		knowledgeID:      knowledgeID,
		mu:               &sync.RWMutex{},
	}
}

// recordInit initializes metric tracking for a QA pair
func (h *HookMetric) recordInit(index int) {
	h.qaPairMetricList[index] = &qaPairMetric{}
}

// recordQaPair records the QA pair data
func (h *HookMetric) recordQaPair(index int, qaPair *types.QAPair) {
	h.qaPairMetricList[index].qaPair = qaPair
}

// recordSearchResult records search results
func (h *HookMetric) recordSearchResult(index int, searchResult []*types.SearchResult) {
	h.qaPairMetricList[index].searchResult = searchResult
}

// recordRerankResult records reranked results
func (h *HookMetric) recordRerankResult(index int, rerankResult []*types.SearchResult) {
	h.qaPairMetricList[index].rerankResult = rerankResult
}

// recordChatResponse records the generated chat response
func (h *HookMetric) recordChatResponse(index int, chatResponse *types.ChatResponse) {
	h.qaPairMetricList[index].chatResponse = chatResponse
}

// recordFinish finalizes metrics for a QA pair
func (h *HookMetric) recordFinish(index int) {
	// Prepare retrieval source: prefer rerank results, fall back to search results
	retrievalSource := h.qaPairMetricList[index].rerankResult
	if len(retrievalSource) == 0 {
		retrievalSource = h.qaPairMetricList[index].searchResult
	}

	// Evaluation ingests a PID-indexed passage slice into one temporary knowledge.
	// Passage processing preserves the slice index as ChunkIndex, so a result from
	// that knowledge has stable PID provenance without guessing from its content.
	// Keep one entry per rank: unknown sources and duplicate PIDs remain explicit
	// misses instead of being removed and compressing the ranking.
	qaPair := h.qaPairMetricList[index].qaPair
	retrievalIDs := evaluationRetrievalIDsWithProvenance(retrievalSource, h.knowledgeID)

	// Get generated text if available
	generatedTexts := ""
	if h.qaPairMetricList[index].chatResponse != nil {
		generatedTexts = h.qaPairMetricList[index].chatResponse.Content
	}

	// Prepare metric input data
	metricInput := &types.MetricInput{
		RetrievalGT:    [][]int{qaPair.PIDs},
		RetrievalIDs:   retrievalIDs,
		GeneratedTexts: generatedTexts,
		GeneratedGT:    qaPair.Answer,
	}

	// Thread-safe append of metrics
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metricResults.Append(metricInput)
}

// MetricResult returns the averaged metric results
func (h *HookMetric) MetricResult() *types.MetricResult {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.metricResults.Avg()
}

// evaluationRetrievalIDsWithProvenance maps one ranked result list to the
// per-rank PID list with the M0 provenance contract: a result counts only
// when it belongs to this evaluation's temporary knowledge and its
// ChunkIndex (the passage slice index, i.e. PID) is valid and unseen;
// unknown sources and duplicates keep their rank as -1.
func evaluationRetrievalIDsWithProvenance(results []*types.SearchResult, knowledgeID string) []int {
	retrievalIDs := make([]int, len(results))
	seen := make(map[int]struct{}, len(results))
	for i, r := range results {
		retrievalIDs[i] = unknownRetrievalID
		if r == nil || r.KnowledgeID != knowledgeID || r.ChunkIndex < 0 {
			continue
		}
		pid := r.ChunkIndex
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		retrievalIDs[i] = pid
	}
	return retrievalIDs
}

// evaluationRankedResultsWithProvenance renders one ranked list for
// per-question persistence: every position keeps rank, score, PID (or -1),
// and its provenance state.
func evaluationRankedResultsWithProvenance(
	results []*types.SearchResult,
	knowledgeID string,
) []types.EvaluationRankedResult {
	ranked := make([]types.EvaluationRankedResult, len(results))
	seen := make(map[int]struct{}, len(results))
	for i, r := range results {
		ranked[i] = types.EvaluationRankedResult{
			Rank:       i + 1,
			PID:        unknownRetrievalID,
			Provenance: types.EvaluationRankProvenanceUnknown,
		}
		if r == nil {
			continue
		}
		ranked[i].Score = r.Score
		if r.KnowledgeID != knowledgeID || r.ChunkIndex < 0 {
			continue
		}
		pid := r.ChunkIndex
		if _, duplicate := seen[pid]; duplicate {
			ranked[i].Provenance = types.EvaluationRankProvenanceDuplicate
			continue
		}
		seen[pid] = struct{}{}
		ranked[i].PID = pid
		ranked[i].Provenance = types.EvaluationRankProvenanceKnown
	}
	return ranked
}

// evaluationGenerationPIDOrder lists the PIDs that actually entered the
// generation stage: known-provenance entries of the effective retrieval
// order (rerank preferred), truncated to the rerank top-k window.
func evaluationGenerationPIDOrder(ranked []types.EvaluationRankedResult, rerankTopK int) []int {
	pids := make([]int, 0, len(ranked))
	for _, entry := range ranked {
		if entry.Provenance != types.EvaluationRankProvenanceKnown {
			continue
		}
		pids = append(pids, entry.PID)
		if rerankTopK > 0 && len(pids) >= rerankTopK {
			break
		}
	}
	return pids
}

// questionResultInput assembles the per-question publication input for one
// completed sample. Callers hold publishMu, so the per-sample metric is the
// entry appended by the immediately preceding recordFinish.
func (h *HookMetric) questionResultInput(
	index int,
	plan *types.EvaluationMetricPlanSnapshot,
	rerankTopK int,
) *types.EvaluationQuestionResultInput {
	tracked := h.qaPairMetricList[index]
	if tracked == nil || tracked.qaPair == nil {
		return nil
	}
	qaPair := tracked.qaPair

	retrievalSource := tracked.rerankResult
	if len(retrievalSource) == 0 {
		retrievalSource = tracked.searchResult
	}
	effectiveRanked := evaluationRankedResultsWithProvenance(retrievalSource, h.knowledgeID)

	var generatedText string
	var promptTokens, completionTokens, totalTokens *int
	if tracked.chatResponse != nil {
		generatedText = tracked.chatResponse.Content
		prompt := tracked.chatResponse.Usage.PromptTokens
		completion := tracked.chatResponse.Usage.CompletionTokens
		total := tracked.chatResponse.Usage.TotalTokens
		promptTokens = &prompt
		completionTokens = &completion
		totalTokens = &total
	}

	var perSample *types.MetricResult
	h.mu.RLock()
	if count := len(h.metricResults.results); count > 0 {
		perSample = h.metricResults.results[count-1]
	}
	h.mu.RUnlock()

	return &types.EvaluationQuestionResultInput{
		SampleIndex:      index,
		QID:              strconv.Itoa(qaPair.QID),
		Question:         qaPair.Question,
		ReferenceAnswer:  qaPair.Answer,
		GroundTruthPIDs:  append([]int(nil), qaPair.PIDs...),
		SearchResults:    evaluationRankedResultsWithProvenance(tracked.searchResult, h.knowledgeID),
		RerankResults:    evaluationRankedResultsWithProvenance(tracked.rerankResult, h.knowledgeID),
		GenerationPIDs:   evaluationGenerationPIDOrder(effectiveRanked, rerankTopK),
		GeneratedText:    generatedText,
		PerSampleMetrics: perSample,
		Observations:     types.EvaluationMetricObservationsFromResult(plan, perSample),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Status:           types.EvaluationQuestionStatusSuccess,
	}
}
