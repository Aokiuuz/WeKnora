package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/modelcache"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type embeddingCacheRepository struct{ db *gorm.DB }

func NewEmbeddingCacheRepository(db *gorm.DB) modelcache.Store {
	return &embeddingCacheRepository{db: db}
}

func (r *embeddingCacheRepository) GetEmbeddingCache(
	ctx context.Context,
	prefix modelcache.CachePrefix,
	hashes []string,
) (map[string]*types.EmbeddingCacheEntry, error) {
	result := make(map[string]*types.EmbeddingCacheEntry)
	if len(hashes) == 0 {
		return result, nil
	}
	if prefix.TenantID == 0 || prefix.ModelID == "" || prefix.ModelFingerprint == "" || prefix.RequestOptionsSHA256 == "" {
		return nil, errors.New("get embedding cache: complete prefix is required")
	}
	now := time.Now().UTC()
	var entries []*types.EmbeddingCacheEntry
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND model_id = ? AND model_fingerprint = ? AND request_options_sha256 = ?",
			prefix.TenantID, prefix.ModelID, prefix.ModelFingerprint, prefix.RequestOptionsSHA256).
		Where("text_sha256 IN ? AND expires_at > ?", hashes, now).
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("get embedding cache: %w", err)
	}
	for _, entry := range entries {
		result[entry.TextSHA256] = entry
	}
	if len(entries) > 0 {
		_ = r.db.WithContext(ctx).Model(&types.EmbeddingCacheEntry{}).
			Where("tenant_id = ? AND model_id = ? AND model_fingerprint = ? AND request_options_sha256 = ? AND text_sha256 IN ?",
				prefix.TenantID, prefix.ModelID, prefix.ModelFingerprint, prefix.RequestOptionsSHA256, hashes).
			Updates(map[string]any{"accessed_at": now, "updated_at": now}).Error
	}
	return result, nil
}

func (r *embeddingCacheRepository) PutEmbeddingCache(ctx context.Context, entries []*types.EmbeddingCacheEntry) error {
	if len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		if entry == nil || entry.TenantID == 0 || entry.ModelID == "" || entry.ModelFingerprint == "" ||
			entry.RequestOptionsSHA256 == "" || entry.TextSHA256 == "" || len(entry.Embedding) == 0 ||
			entry.Dimension <= 0 || entry.ChecksumSHA256 == "" || entry.ExpiresAt.IsZero() {
			return errors.New("put embedding cache: complete validated entry is required")
		}
	}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "model_id"}, {Name: "model_fingerprint"},
			{Name: "request_options_sha256"}, {Name: "text_sha256"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"embedding", "dimension", "checksum_sha256", "expires_at", "accessed_at", "updated_at",
		}),
	}).Create(&entries).Error; err != nil {
		return fmt.Errorf("put embedding cache: %w", err)
	}
	return nil
}

func (r *embeddingCacheRepository) DeleteExpiredEmbeddingCache(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		return 0, errors.New("delete expired embedding cache: positive limit is required")
	}
	var entries []*types.EmbeddingCacheEntry
	if err := r.db.WithContext(ctx).Where("expires_at <= ?", time.Now().UTC()).
		Order("expires_at ASC").Limit(limit).Find(&entries).Error; err != nil {
		return 0, fmt.Errorf("list expired embedding cache: %w", err)
	}
	if len(entries) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Delete(&entries)
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired embedding cache: %w", result.Error)
	}
	return result.RowsAffected, nil
}

var _ modelcache.Store = (*embeddingCacheRepository)(nil)
