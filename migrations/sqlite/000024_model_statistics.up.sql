CREATE TABLE IF NOT EXISTS embedding_cache_events (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    model_id TEXT NOT NULL,
    hit_items INTEGER NOT NULL,
    miss_items INTEGER NOT NULL,
    bypass_items INTEGER NOT NULL,
    occurred_at DATETIME NOT NULL,
    CONSTRAINT embedding_cache_events_counts_check CHECK (
        hit_items >= 0 AND miss_items >= 0 AND bypass_items >= 0
        AND hit_items + miss_items + bypass_items > 0
    )
);

CREATE INDEX IF NOT EXISTS idx_embedding_cache_events_tenant_model_time
    ON embedding_cache_events (tenant_id, model_id, occurred_at DESC);
