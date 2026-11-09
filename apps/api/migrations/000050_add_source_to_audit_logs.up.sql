-- Plan 023 item 1.6: where an audit row came from, so the Connected apps screen
-- can list what each OAuth client changed.
ALTER TABLE audit_logs
    ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'web',
    ADD COLUMN client_id VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX idx_audit_logs_user_source_created
    ON audit_logs (user_id, source, created_at DESC);

-- Backfill from the "via" and "client_id" keys MCP writes put in changes.
-- changes is TEXT and is '' when a caller passes no changes (logout does), so
-- the jsonb cast is guarded: CASE only casts values that look like an object.
UPDATE audit_logs
    SET source = 'mcp',
        client_id = COALESCE(changes::jsonb->>'client_id', '')
    WHERE CASE WHEN changes LIKE '{%' THEN changes::jsonb->>'via' END = 'mcp';
