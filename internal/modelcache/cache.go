package modelcache

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/modelobs"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	"golang.org/x/sync/singleflight"
)

const (
	DefaultTTL          = 30 * 24 * time.Hour
	CleanupInterval     = 24 * time.Hour
	CleanupBatchSize    = 500
	CleanupRoundTimeout = 30 * time.Second
	cacheWriteTimeout   = 2 * time.Second
)

// CachePrefix is the non-text portion of the tenant-isolated composite key.
type CachePrefix struct {
	TenantID             uint64
	ModelID              string
	ModelFingerprint     string
	RequestOptionsSHA256 string
}

// Store persists content-addressed vectors.
type Store interface {
	GetEmbeddingCache(context.Context, CachePrefix, []string) (map[string]*types.EmbeddingCacheEntry, error)
	PutEmbeddingCache(context.Context, []*types.EmbeddingCacheEntry) error
	DeleteExpiredEmbeddingCache(context.Context, int) (int64, error)
}

// Coordinator owns the process-wide singleflight group shared by every wrapper.
type Coordinator struct {
	store     Store
	ttl       time.Duration
	requests  singleflight.Group
	startOnce sync.Once
	stopOnce  sync.Once
	started   chan struct{}
	stop      chan struct{}
	done      chan struct{}
}

func NewCoordinator(store Store) *Coordinator {
	return &Coordinator{
		store: store, ttl: DefaultTTL,
		started: make(chan struct{}), stop: make(chan struct{}), done: make(chan struct{}),
	}
}

// CleanupOnce deletes expired rows in bounded batches for at most one cleanup round.
func (c *Coordinator) CleanupOnce(ctx context.Context) error {
	if c == nil || c.store == nil {
		return nil
	}
	roundCtx, cancel := context.WithTimeout(ctx, CleanupRoundTimeout)
	defer cancel()
	for {
		count, err := c.store.DeleteExpiredEmbeddingCache(roundCtx, CleanupBatchSize)
		if err != nil {
			return err
		}
		if count < CleanupBatchSize {
			return nil
		}
	}
}

// StartCleaner runs one cleanup at startup and then every cleanup interval.
func (c *Coordinator) StartCleaner(ctx context.Context) {
	if c == nil {
		return
	}
	c.startOnce.Do(func() {
		close(c.started)
		go func() {
			defer close(c.done)
			_ = c.CleanupOnce(context.WithoutCancel(ctx))
			ticker := time.NewTicker(CleanupInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_ = c.CleanupOnce(context.Background())
				case <-c.stop:
					return
				}
			}
		}()
	})
}

// StopCleaner terminates the process-lifetime cleanup loop.
func (c *Coordinator) StopCleaner() {
	if c == nil {
		return
	}
	select {
	case <-c.started:
	default:
		return
	}
	c.stopOnce.Do(func() { close(c.stop) })
	<-c.done
}

func (c *Coordinator) Wrap(model *types.Model, inner embedding.Embedder) embedding.Embedder {
	if c == nil || c.store == nil || model == nil || inner == nil {
		return inner
	}
	return &cachedEmbedder{coordinator: c, model: model, inner: inner}
}

type cachedEmbedder struct {
	coordinator *Coordinator
	model       *types.Model
	inner       embedding.Embedder
}

func (e *cachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	result, err := e.cachedBatch(ctx, []string{text}, func(callCtx context.Context, missing []string) ([][]float32, error) {
		vector, err := e.inner.Embed(callCtx, missing[0])
		if err != nil {
			return nil, err
		}
		return [][]float32{vector}, nil
	})
	if err != nil {
		return nil, err
	}
	return result[0], nil
}

func (e *cachedEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	return e.cachedBatch(ctx, texts, e.inner.BatchEmbed)
}

func (e *cachedEmbedder) BatchEmbedWithPool(ctx context.Context, _ embedding.Embedder, texts []string) ([][]float32, error) {
	return e.cachedBatch(ctx, texts, func(callCtx context.Context, missing []string) ([][]float32, error) {
		return e.inner.BatchEmbedWithPool(callCtx, e.inner, missing)
	})
}

func (e *cachedEmbedder) GetModelName() string { return e.inner.GetModelName() }
func (e *cachedEmbedder) GetDimensions() int   { return e.inner.GetDimensions() }
func (e *cachedEmbedder) GetModelID() string   { return e.inner.GetModelID() }

func (e *cachedEmbedder) cachedBatch(
	ctx context.Context,
	texts []string,
	provider func(context.Context, []string) ([][]float32, error),
) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		tenantID = e.model.TenantID
	}
	prefix := CachePrefix{
		TenantID: tenantID, ModelID: e.model.ID,
		ModelFingerprint:     EmbeddingModelFingerprint(e.model),
		RequestOptionsSHA256: requestOptionsSHA256(e.inner.GetDimensions()),
	}
	uniqueTexts := make([]string, 0, len(texts))
	uniqueHashes := make([]string, 0, len(texts))
	hashToIndex := make(map[string]int, len(texts))
	originalHashes := make([]string, len(texts))
	for i, text := range texts {
		hash := sha256Hex([]byte(text))
		originalHashes[i] = hash
		if _, exists := hashToIndex[hash]; exists {
			continue
		}
		hashToIndex[hash] = len(uniqueTexts)
		uniqueTexts = append(uniqueTexts, text)
		uniqueHashes = append(uniqueHashes, hash)
	}

	cached, lookupErr := e.coordinator.store.GetEmbeddingCache(ctx, prefix, uniqueHashes)
	lookupStatus := types.ApplicationCacheStatusMiss
	if lookupErr != nil {
		cached = map[string]*types.EmbeddingCacheEntry{}
		lookupStatus = types.ApplicationCacheStatusBypass
	}
	uniqueVectors := make(map[string][]float32, len(uniqueTexts))
	missingTexts := make([]string, 0)
	missingHashes := make([]string, 0)
	for i, hash := range uniqueHashes {
		if entry := cached[hash]; entry != nil {
			if vector, err := decodeCacheVector(entry, e.inner.GetDimensions()); err == nil {
				uniqueVectors[hash] = vector
				continue
			}
		}
		missingTexts = append(missingTexts, uniqueTexts[i])
		missingHashes = append(missingHashes, hash)
	}
	if len(missingTexts) > 0 {
		batchKey := singleflightBatchKey(prefix, missingHashes)
		value, err, _ := e.coordinator.requests.Do(batchKey, func() (any, error) {
			callCtx := modelobs.WithApplicationCacheStatus(ctx, lookupStatus)
			vectors, err := provider(callCtx, missingTexts)
			if err != nil {
				return nil, err
			}
			entries, err := buildCacheEntries(prefix, missingHashes, vectors, e.inner.GetDimensions(), e.coordinator.ttl)
			if err != nil {
				return nil, err
			}
			writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cacheWriteTimeout)
			defer cancel()
			_ = e.coordinator.store.PutEmbeddingCache(writeCtx, entries)
			return vectors, nil
		})
		if err != nil {
			return nil, err
		}
		vectors := value.([][]float32)
		if len(vectors) != len(missingHashes) {
			return nil, errors.New("embedding cache: provider result count mismatch")
		}
		for i, hash := range missingHashes {
			uniqueVectors[hash] = append([]float32(nil), vectors[i]...)
		}
	}
	result := make([][]float32, len(texts))
	for i, hash := range originalHashes {
		vector, exists := uniqueVectors[hash]
		if !exists {
			return nil, errors.New("embedding cache: missing restored vector")
		}
		result[i] = append([]float32(nil), vector...)
	}
	return result, nil
}

func EmbeddingModelFingerprint(model *types.Model) string {
	if model == nil {
		return sha256Hex(nil)
	}
	encoded, _ := json.Marshal(struct {
		ID          string                    `json:"id"`
		Name        string                    `json:"name"`
		Source      types.ModelSource         `json:"source"`
		Provider    string                    `json:"provider"`
		BaseURL     string                    `json:"base_url"`
		Embedding   types.EmbeddingParameters `json:"embedding"`
		Extra       map[string]string         `json:"extra"`
		HeaderNames []string                  `json:"header_names"`
	}{
		ID: model.ID, Name: model.Name, Source: model.Source, Provider: model.Parameters.Provider,
		BaseURL: model.Parameters.BaseURL, Embedding: model.Parameters.EmbeddingParameters,
		Extra: model.Parameters.ExtraConfig, HeaderNames: sortedMapKeys(model.Parameters.CustomHeaders),
	})
	return sha256Hex(encoded)
}

func requestOptionsSHA256(dimensions int) string {
	return sha256Hex([]byte(fmt.Sprintf("schema=1;encoding=float32-le;dimensions=%d", dimensions)))
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, strings.ToLower(key))
	}
	sort.Strings(keys)
	return keys
}

func singleflightBatchKey(prefix CachePrefix, hashes []string) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s", prefix.TenantID, prefix.ModelID,
		prefix.ModelFingerprint, prefix.RequestOptionsSHA256, strings.Join(hashes, ","))
}

func buildCacheEntries(prefix CachePrefix, hashes []string, vectors [][]float32, expectedDimension int, ttl time.Duration) ([]*types.EmbeddingCacheEntry, error) {
	if len(vectors) != len(hashes) {
		return nil, errors.New("embedding cache: provider result count mismatch")
	}
	now := time.Now().UTC()
	entries := make([]*types.EmbeddingCacheEntry, len(vectors))
	for i, vector := range vectors {
		if err := validateVector(vector, expectedDimension); err != nil {
			return nil, err
		}
		encoded := encodeVector(vector)
		entries[i] = &types.EmbeddingCacheEntry{
			TenantID: prefix.TenantID, ModelID: prefix.ModelID, ModelFingerprint: prefix.ModelFingerprint,
			RequestOptionsSHA256: prefix.RequestOptionsSHA256, TextSHA256: hashes[i], Embedding: encoded,
			Dimension: len(vector), ChecksumSHA256: sha256Hex(encoded), ExpiresAt: now.Add(ttl),
			AccessedAt: now, CreatedAt: now, UpdatedAt: now,
		}
	}
	return entries, nil
}

func validateVector(vector []float32, expectedDimension int) error {
	if len(vector) == 0 {
		return errors.New("embedding cache: empty provider vector")
	}
	if expectedDimension > 0 && len(vector) != expectedDimension {
		return errors.New("embedding cache: provider dimension mismatch")
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("embedding cache: non-finite provider vector")
		}
	}
	return nil
}

func encodeVector(vector []float32) []byte {
	encoded := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(encoded[i*4:], math.Float32bits(value))
	}
	return encoded
}

func decodeCacheVector(entry *types.EmbeddingCacheEntry, expectedDimension int) ([]float32, error) {
	if entry == nil || entry.Dimension <= 0 || len(entry.Embedding) != entry.Dimension*4 ||
		(expectedDimension > 0 && entry.Dimension != expectedDimension) || sha256Hex(entry.Embedding) != entry.ChecksumSHA256 {
		return nil, errors.New("embedding cache: invalid cached vector")
	}
	vector := make([]float32, entry.Dimension)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(entry.Embedding[i*4:]))
	}
	if err := validateVector(vector, expectedDimension); err != nil {
		return nil, err
	}
	return vector, nil
}

func sha256Hex(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
