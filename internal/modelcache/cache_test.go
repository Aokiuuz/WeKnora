package modelcache

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cacheStore struct {
	mu      sync.Mutex
	entries map[string]*types.EmbeddingCacheEntry
	getErr  error
	putErr  error
	events  []*types.EmbeddingCacheEvent
}

func cacheStoreKey(prefix CachePrefix, hash string) string {
	return fmt.Sprintf("%d/%s/%s/%s/%s", prefix.TenantID, prefix.ModelID, prefix.ModelFingerprint, prefix.RequestOptionsSHA256, hash)
}

func (s *cacheStore) GetEmbeddingCache(_ context.Context, prefix CachePrefix, hashes []string) (map[string]*types.EmbeddingCacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	result := map[string]*types.EmbeddingCacheEntry{}
	for _, hash := range hashes {
		if entry := s.entries[cacheStoreKey(prefix, hash)]; entry != nil {
			copy := *entry
			result[hash] = &copy
		}
	}
	return result, nil
}
func (s *cacheStore) PutEmbeddingCache(_ context.Context, entries []*types.EmbeddingCacheEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putErr != nil {
		return s.putErr
	}
	if s.entries == nil {
		s.entries = map[string]*types.EmbeddingCacheEntry{}
	}
	for _, entry := range entries {
		copy := *entry
		copy.Embedding = append([]byte(nil), entry.Embedding...)
		prefix := CachePrefix{TenantID: entry.TenantID, ModelID: entry.ModelID, ModelFingerprint: entry.ModelFingerprint, RequestOptionsSHA256: entry.RequestOptionsSHA256}
		s.entries[cacheStoreKey(prefix, entry.TextSHA256)] = &copy
	}
	return nil
}

func (s *cacheStore) RecordEmbeddingCacheEvent(_ context.Context, event *types.EmbeddingCacheEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *event
	s.events = append(s.events, &copy)
	return nil
}

func TestEmbeddingCacheReadAndWriteFailuresAreFailOpen(t *testing.T) {
	store := &cacheStore{getErr: errors.New("read unavailable"), putErr: errors.New("write unavailable")}
	provider := &countingEmbedder{}
	wrapped := NewCoordinator(store).Wrap(&types.Model{ID: "embedding-1", TenantID: 7}, provider)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	result, err := wrapped.BatchEmbed(ctx, []string{"alpha", "alpha"})
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, result[0], result[1])
	require.Len(t, provider.batchInputs, 1)
}

func TestEmbeddingCacheRejectsNonFiniteProviderVector(t *testing.T) {
	provider := &invalidEmbedder{}
	wrapped := NewCoordinator(&cacheStore{}).Wrap(&types.Model{ID: "embedding-1", TenantID: 7}, provider)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	_, err := wrapped.BatchEmbed(ctx, []string{"alpha"})
	require.ErrorContains(t, err, "non-finite")
}

type invalidEmbedder struct{ countingEmbedder }

func (*invalidEmbedder) BatchEmbed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{float32(math.NaN()), 1}}, nil
}

func TestEmbeddingCacheSeparatesTenants(t *testing.T) {
	store := &cacheStore{}
	provider := &countingEmbedder{}
	wrapped := NewCoordinator(store).Wrap(&types.Model{ID: "embedding-1", TenantID: 7}, provider)
	for _, tenantID := range []uint64{7, 8} {
		ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
		_, err := wrapped.BatchEmbed(ctx, []string{"same"})
		require.NoError(t, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	assert.Len(t, provider.batchInputs, 2)
}
func (*cacheStore) DeleteExpiredEmbeddingCache(context.Context, int) (int64, error) { return 0, nil }

type countingEmbedder struct {
	mu          sync.Mutex
	batchInputs [][]string
}

func (e *countingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	return []float32{float32(len(text)), 1}, nil
}
func (e *countingEmbedder) BatchEmbed(_ context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.batchInputs = append(e.batchInputs, append([]string(nil), texts...))
	e.mu.Unlock()
	result := make([][]float32, len(texts))
	for i, text := range texts {
		result[i] = []float32{float32(len(text)), float32(i + 1)}
	}
	return result, nil
}
func (e *countingEmbedder) BatchEmbedWithPool(ctx context.Context, _ embedding.Embedder, texts []string) ([][]float32, error) {
	return e.BatchEmbed(ctx, texts)
}
func (*countingEmbedder) GetModelName() string { return "embedding" }
func (*countingEmbedder) GetDimensions() int   { return 2 }
func (*countingEmbedder) GetModelID() string   { return "embedding-1" }

func TestBatchEmbeddingCacheDeduplicatesMissesAndRestoresOrder(t *testing.T) {
	store := &cacheStore{}
	coordinator := NewCoordinator(store)
	provider := &countingEmbedder{}
	model := &types.Model{ID: "embedding-1", TenantID: 7, Name: "embedding", Type: types.ModelTypeEmbedding}
	wrapped := coordinator.Wrap(model, provider)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	first, err := wrapped.BatchEmbed(ctx, []string{"alpha", "beta", "alpha"})
	require.NoError(t, err)
	require.Len(t, provider.batchInputs, 1)
	assert.Equal(t, []string{"alpha", "beta"}, provider.batchInputs[0])
	assert.Equal(t, first[0], first[2])

	second, err := wrapped.BatchEmbed(ctx, []string{"alpha", "beta", "alpha"})
	require.NoError(t, err)
	require.Len(t, provider.batchInputs, 1, "hot cache must send zero provider input items")
	assert.Equal(t, first, second)
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.entries, 2)
	require.Len(t, store.events, 2)
	assert.Equal(t, int64(2), store.events[0].MissItems)
	assert.Equal(t, int64(2), store.events[1].HitItems)
}

func TestEmbeddingCacheSingleflightIsSharedAcrossWrappers(t *testing.T) {
	store := &cacheStore{}
	coordinator := NewCoordinator(store)
	provider := &countingEmbedder{}
	model := &types.Model{ID: "embedding-1", TenantID: 7, Name: "embedding", Type: types.ModelTypeEmbedding}
	first := coordinator.Wrap(model, provider)
	second := coordinator.Wrap(model, provider)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	for _, embedder := range []embedding.Embedder{first, second} {
		go func(item embedding.Embedder) {
			defer wait.Done()
			<-start
			_, _ = item.BatchEmbed(ctx, []string{"same"})
		}(embedder)
	}
	close(start)
	wait.Wait()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	assert.Len(t, provider.batchInputs, 1)
}
