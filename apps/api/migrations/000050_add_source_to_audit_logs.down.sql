DROP INDEX IF EXISTS idx_audit_logs_user_source_created;
ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS client_id,
    DROP COLUMN IF EXISTS source;
