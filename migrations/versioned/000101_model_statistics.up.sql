CREATE TABLE IF NOT EXISTS embedding_cache_events (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    model_id VARCHAR(64) NOT NULL,
    hit_items BIGINT NOT NULL,
    miss_items BIGINT NOT NULL,
    bypass_items BIGINT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT embedding_cache_events_counts_check CHECK (
        hit_items >= 0 AND miss_items >= 0 AND bypass_items >= 0
        AND hit_items + miss_items + bypass_items > 0
    )
);

CREATE INDEX IF NOT EXISTS idx_embedding_cache_events_tenant_model_time
    ON embedding_cache_events (tenant_id, model_id, occurred_at DESC);
