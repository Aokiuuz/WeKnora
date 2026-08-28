-- Mirrors versioned migration 000090_evaluation_tasks.

CREATE TABLE IF NOT EXISTS evaluation_tasks (
    id                     TEXT PRIMARY KEY,
    tenant_id              INTEGER NOT NULL,
    dataset_id             TEXT NOT NULL,
    status                 INTEGER NOT NULL,
    start_time             DATETIME NOT NULL,
    end_time               DATETIME,
    total                  INTEGER NOT NULL DEFAULT 0,
    finished               INTEGER NOT NULL DEFAULT 0,
    err_msg                TEXT NOT NULL DEFAULT '',
    cleanup_errors         TEXT NOT NULL DEFAULT '[]',
    params                 TEXT NOT NULL DEFAULT '{}',
    metric                 TEXT,
    temporary_kb_id        TEXT NOT NULL,
    temporary_knowledge_id TEXT,
    owner_id               TEXT NOT NULL,
    lease_expires_at       DATETIME NOT NULL,
    heartbeat_at           DATETIME NOT NULL,
    version                INTEGER NOT NULL DEFAULT 1,
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at             DATETIME
);

CREATE INDEX IF NOT EXISTS idx_evaluation_tasks_tenant_started
    ON evaluation_tasks (tenant_id, start_time DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_evaluation_tasks_tenant_status
    ON evaluation_tasks (tenant_id, status, start_time DESC);

CREATE INDEX IF NOT EXISTS idx_evaluation_tasks_active_lease
    ON evaluation_tasks (status, lease_expires_at);
