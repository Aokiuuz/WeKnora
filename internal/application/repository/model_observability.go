package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/modelobs"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type modelObservabilityRepository struct{ db *gorm.DB }

// NewModelObservabilityRepository creates the model call ledger and pricing store.
func NewModelObservabilityRepository(db *gorm.DB) modelobs.Store {
	return &modelObservabilityRepository{db: db}
}

func (r *modelObservabilityRepository) StartModelCall(ctx context.Context, record *types.ModelCallRecord) error {
	if record == nil || record.ID == "" || record.TenantID == 0 || record.ModelID == "" ||
		record.Purpose == "" || record.Operation == "" || record.StartedAt.IsZero() {
		return errors.New("start model call: id, tenant, model, purpose, operation, and started_at are required")
	}
	if record.Status != types.ModelCallStatusStarted {
		return errors.New("start model call: status must be started")
	}
	if err := validateEvaluationTaskJSONObject(record.ModelSnapshot, false); err != nil {
		return fmt.Errorf("start model call: model_snapshot: %w", err)
	}
	record.StartedAt = record.StartedAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.UpdatedAt.UTC()
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("start model call %s: %w", record.ID, err)
	}
	return nil
}

func (r *modelObservabilityRepository) CompleteModelCall(
	ctx context.Context,
	completion types.ModelCallCompletion,
) error {
	if completion.ID == "" || completion.EndedAt.IsZero() || completion.DurationMs < 0 {
		return errors.New("complete model call: id, ended_at, and non-negative duration are required")
	}
	switch completion.Status {
	case types.ModelCallStatusSuccess, types.ModelCallStatusError, types.ModelCallStatusCanceled:
	default:
		return errors.New("complete model call: unsupported terminal status")
	}
	endedAt := completion.EndedAt.UTC()
	result := r.db.WithContext(ctx).Model(&types.ModelCallRecord{}).
		Where("id = ? AND status = ?", completion.ID, types.ModelCallStatusStarted).
		Updates(map[string]any{
			"ended_at":                    endedAt,
			"duration_ms":                 completion.DurationMs,
			"status":                      completion.Status,
			"error_code":                  completion.ErrorCode,
			"prompt_tokens":               completion.PromptTokens,
			"completion_tokens":           completion.CompletionTokens,
			"total_tokens":                completion.TotalTokens,
			"provider_cache_status":       completion.ProviderCacheStatus,
			"provider_cache_read_tokens":  completion.ProviderCacheReadTokens,
			"provider_cache_write_tokens": completion.ProviderCacheWriteTokens,
			"provider_cache_miss_tokens":  completion.ProviderCacheMissTokens,
			"application_cache_status":    completion.ApplicationCacheStatus,
			"cost_microunits":             completion.CostMicrounits,
			"accounting_complete":         completion.AccountingComplete,
			"updated_at":                  endedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("complete model call %s: %w", completion.ID, result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	if result.RowsAffected > 1 {
		return fmt.Errorf("complete model call %s: invariant violation", completion.ID)
	}
	var existing types.ModelCallRecord
	if err := r.db.WithContext(ctx).First(&existing, "id = ?", completion.ID).Error; err != nil {
		return fmt.Errorf("complete model call %s: %w", completion.ID, err)
	}
	if existing.Status == types.ModelCallStatusStarted {
		return fmt.Errorf("complete model call %s: concurrent state conflict", completion.ID)
	}
	return nil
}

func (r *modelObservabilityRepository) EffectiveModelPrice(
	ctx context.Context,
	tenantID uint64,
	modelID string,
	at time.Time,
) (*types.ModelPriceVersion, error) {
	if tenantID == 0 || modelID == "" || at.IsZero() {
		return nil, errors.New("resolve model price: tenant, model, and time are required")
	}
	var price types.ModelPriceVersion
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ? AND valid_from <= ?", tenantID, modelID, at.UTC()).
		Where("valid_to IS NULL OR valid_to > ?", at.UTC()).
		Order("valid_from DESC").
		Order("id DESC").
		First(&price).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve model price %s: %w", modelID, err)
	}
	price.ValidFrom = price.ValidFrom.UTC()
	if price.ValidTo != nil {
		value := price.ValidTo.UTC()
		price.ValidTo = &value
	}
	price.CreatedAt = price.CreatedAt.UTC()
	return &price, nil
}

func (r *modelObservabilityRepository) CreateModelPrice(
	ctx context.Context,
	price *types.ModelPriceVersion,
) error {
	if price == nil || price.TenantID == 0 || price.ModelID == "" || price.ValidFrom.IsZero() {
		return errors.New("create model price: tenant, model, and valid_from are required")
	}
	price.Currency = strings.ToUpper(strings.TrimSpace(price.Currency))
	if !isCurrencyCode(price.Currency) || price.InputMicrounitsPerMillion < 0 || price.OutputMicrounitsPerMillion < 0 {
		return errors.New("create model price: currency must have three letters and prices must be non-negative")
	}
	price.ValidFrom = price.ValidFrom.UTC()
	if price.ValidTo != nil {
		value := price.ValidTo.UTC()
		if !value.After(price.ValidFrom) {
			return errors.New("create model price: valid_to must be after valid_from")
		}
		price.ValidTo = &value
	}
	if price.ID == "" {
		price.ID = uuid.NewString()
	}
	if price.CreatedAt.IsZero() {
		price.CreatedAt = time.Now().UTC()
	} else {
		price.CreatedAt = price.CreatedAt.UTC()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&types.ModelPriceVersion{}).
			Where("tenant_id = ? AND model_id = ?", price.TenantID, price.ModelID).
			Where("valid_to IS NULL OR valid_to > ?", price.ValidFrom)
		if price.ValidTo != nil {
			query = query.Where("valid_from < ?", *price.ValidTo)
		}
		var overlap int64
		if err := query.Count(&overlap).Error; err != nil {
			return fmt.Errorf("check model price overlap: %w", err)
		}
		if overlap > 0 {
			return errors.New("create model price: validity window overlaps an existing version")
		}
		if err := tx.Create(price).Error; err != nil {
			return fmt.Errorf("create model price: %w", err)
		}
		return nil
	})
}

func isCurrencyCode(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func (r *modelObservabilityRepository) ListModelPrices(
	ctx context.Context,
	tenantID uint64,
	modelID string,
) ([]*types.ModelPriceVersion, error) {
	if tenantID == 0 || modelID == "" {
		return nil, errors.New("list model prices: tenant and model are required")
	}
	var prices []*types.ModelPriceVersion
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ?", tenantID, modelID).
		Order("valid_from DESC").Order("id DESC").Find(&prices).Error; err != nil {
		return nil, fmt.Errorf("list model prices %s: %w", modelID, err)
	}
	for _, price := range prices {
		price.ValidFrom = price.ValidFrom.UTC()
		if price.ValidTo != nil {
			value := price.ValidTo.UTC()
			price.ValidTo = &value
		}
		price.CreatedAt = price.CreatedAt.UTC()
	}
	return prices, nil
}

var _ modelobs.Store = (*modelObservabilityRepository)(nil)
