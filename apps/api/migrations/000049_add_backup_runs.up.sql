-- Plan 023 1.7: heartbeat written by the backup service after each pg_dump
-- attempt. The API only reads it (GET /system/status -> last_backup_at).
CREATE TABLE backup_runs (
    id          BIGSERIAL PRIMARY KEY,
    started_at  TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    status      VARCHAR(10) NOT NULL CHECK (status IN ('ok', 'failed')),
    size_bytes  BIGINT,
    error       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_backup_runs_status_finished ON backup_runs (status, finished_at DESC);
