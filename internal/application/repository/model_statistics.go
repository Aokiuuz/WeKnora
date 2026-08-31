package repository

import (
	"context"
	"fmt"
	"sort"

	"github.com/Tencent/WeKnora/internal/modelstats"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

type modelStatisticsRepository struct {
	db     *gorm.DB
	prices *modelObservabilityRepository
}

func NewModelStatisticsRepository(db *gorm.DB) modelstats.Store {
	return &modelStatisticsRepository{db: db, prices: NewModelObservabilityRepository(db)}
}

type modelUsageRow struct {
	ModelID                  string
	CallCount                int64
	SuccessCalls             int64
	ErrorCalls               int64
	CanceledCalls            int64
	UsageReportedCalls       int64
	UsageUnreportedCalls     int64
	AccountingCompleteCalls  int64
	UnpricedCalls            int64
	PromptTokens             int64
	CompletionTokens         int64
	TotalTokens              int64
	AverageDurationMs        float64
	ProviderCacheReadTokens  int64
	ProviderCacheWriteTokens int64
	ProviderCacheMissTokens  int64
}

type modelCostRow struct {
	ModelID        string
	Currency       string
	CostMicrounits int64
}

type applicationCacheRow struct {
	ModelID     string
	HitItems    int64
	MissItems   int64
	BypassItems int64
}

func (r *modelStatisticsRepository) QueryModelUsage(
	ctx context.Context,
	query types.ModelUsageQuery,
) ([]types.ModelUsageStatistics, error) {
	if query.TenantID == 0 || query.From.IsZero() || query.To.IsZero() || !query.From.Before(query.To) {
		return nil, fmt.Errorf("query model usage: valid tenant and interval are required")
	}
	base := r.db.WithContext(ctx).Table("model_call_records").
		Where("tenant_id = ? AND started_at >= ? AND started_at < ? AND deleted_at IS NULL",
			query.TenantID, query.From.UTC(), query.To.UTC())
	if len(query.ModelIDs) > 0 {
		base = base.Where("model_id IN ?", query.ModelIDs)
	}
	var usageRows []modelUsageRow
	err := base.Select(`
		model_id,
		COUNT(*) AS call_count,
		SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_calls,
		SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_calls,
		SUM(CASE WHEN status = 'canceled' THEN 1 ELSE 0 END) AS canceled_calls,
		SUM(CASE WHEN total_tokens IS NOT NULL THEN 1 ELSE 0 END) AS usage_reported_calls,
		SUM(CASE WHEN total_tokens IS NULL THEN 1 ELSE 0 END) AS usage_unreported_calls,
		SUM(CASE WHEN accounting_complete THEN 1 ELSE 0 END) AS accounting_complete_calls,
		SUM(CASE WHEN total_tokens IS NOT NULL AND cost_microunits IS NULL THEN 1 ELSE 0 END) AS unpriced_calls,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
		COALESCE(SUM(total_tokens), 0) AS total_tokens,
		COALESCE(AVG(duration_ms), 0) AS average_duration_ms,
		COALESCE(SUM(provider_cache_read_tokens), 0) AS provider_cache_read_tokens,
		COALESCE(SUM(provider_cache_write_tokens), 0) AS provider_cache_write_tokens,
		COALESCE(SUM(provider_cache_miss_tokens), 0) AS provider_cache_miss_tokens`).
		Group("model_id").Scan(&usageRows).Error
	if err != nil {
		return nil, fmt.Errorf("query model usage aggregates: %w", err)
	}

	byModel := make(map[string]*types.ModelUsageStatistics, len(usageRows))
	for _, row := range usageRows {
		observedProvider := row.ProviderCacheReadTokens + row.ProviderCacheMissTokens
		stat := &types.ModelUsageStatistics{
			ModelID: row.ModelID, CallCount: row.CallCount, SuccessCalls: row.SuccessCalls,
			ErrorCalls: row.ErrorCalls, CanceledCalls: row.CanceledCalls,
			UsageReportedCalls: row.UsageReportedCalls, UsageUnreportedCalls: row.UsageUnreportedCalls,
			AccountingCompleteCalls: row.AccountingCompleteCalls, UnpricedCalls: row.UnpricedCalls,
			PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
			AverageDurationMs: row.AverageDurationMs, Costs: []types.ModelCostTotal{},
			ProviderCache: types.ProviderCacheStatistics{
				ReadTokens: row.ProviderCacheReadTokens, WriteTokens: row.ProviderCacheWriteTokens,
				MissTokens: row.ProviderCacheMissTokens, ObservedTokens: observedProvider,
				HitRate: ratioPointer(row.ProviderCacheReadTokens, observedProvider),
			},
		}
		byModel[row.ModelID] = stat
	}

	costQuery := r.db.WithContext(ctx).Table("model_call_records").
		Where("tenant_id = ? AND started_at >= ? AND started_at < ? AND deleted_at IS NULL",
			query.TenantID, query.From.UTC(), query.To.UTC()).
		Where("cost_microunits IS NOT NULL AND currency <> ''")
	if len(query.ModelIDs) > 0 {
		costQuery = costQuery.Where("model_id IN ?", query.ModelIDs)
	}
	var costRows []modelCostRow
	if err := costQuery.Select("model_id, currency, SUM(cost_microunits) AS cost_microunits").
		Group("model_id, currency").Scan(&costRows).Error; err != nil {
		return nil, fmt.Errorf("query model cost aggregates: %w", err)
	}
	for _, row := range costRows {
		stat := ensureUsageStatistic(byModel, row.ModelID)
		stat.Costs = append(stat.Costs, types.ModelCostTotal{Currency: row.Currency, CostMicrounits: row.CostMicrounits})
	}

	cacheQuery := r.db.WithContext(ctx).Table("embedding_cache_events").
		Where("tenant_id = ? AND occurred_at >= ? AND occurred_at < ?", query.TenantID, query.From.UTC(), query.To.UTC())
	if len(query.ModelIDs) > 0 {
		cacheQuery = cacheQuery.Where("model_id IN ?", query.ModelIDs)
	}
	var cacheRows []applicationCacheRow
	if err := cacheQuery.Select(`model_id, COALESCE(SUM(hit_items), 0) AS hit_items,
		COALESCE(SUM(miss_items), 0) AS miss_items, COALESCE(SUM(bypass_items), 0) AS bypass_items`).
		Group("model_id").Scan(&cacheRows).Error; err != nil {
		return nil, fmt.Errorf("query application cache aggregates: %w", err)
	}
	for _, row := range cacheRows {
		stat := ensureUsageStatistic(byModel, row.ModelID)
		observed := row.HitItems + row.MissItems
		stat.ApplicationCache = types.ApplicationCacheStatistics{
			HitItems: row.HitItems, MissItems: row.MissItems, BypassItems: row.BypassItems,
			ObservedItems: observed, HitRate: ratioPointer(row.HitItems, observed),
		}
	}

	result := make([]types.ModelUsageStatistics, 0, len(byModel))
	for _, stat := range byModel {
		sort.Slice(stat.Costs, func(i, j int) bool { return stat.Costs[i].Currency < stat.Costs[j].Currency })
		result = append(result, *stat)
	}
	return result, nil
}

func (r *modelStatisticsRepository) CreateModelPrice(ctx context.Context, price *types.ModelPriceVersion) error {
	return r.prices.CreateModelPrice(ctx, price)
}

func (r *modelStatisticsRepository) ListModelPrices(
	ctx context.Context,
	tenantID uint64,
	modelID string,
) ([]*types.ModelPriceVersion, error) {
	return r.prices.ListModelPrices(ctx, tenantID, modelID)
}

func ensureUsageStatistic(
	values map[string]*types.ModelUsageStatistics,
	modelID string,
) *types.ModelUsageStatistics {
	if value := values[modelID]; value != nil {
		return value
	}
	value := &types.ModelUsageStatistics{ModelID: modelID, Costs: []types.ModelCostTotal{}}
	values[modelID] = value
	return value
}

func ratioPointer(numerator, denominator int64) *float64 {
	if denominator <= 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

var _ modelstats.Store = (*modelStatisticsRepository)(nil)
