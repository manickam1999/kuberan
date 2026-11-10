-- Plan 023 items 1.3/1.4: free-text notes, the origin of each row, and
-- trigram search over the description and notes.
ALTER TABLE transactions
    ADD COLUMN notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'web',
    ADD COLUMN source_client_id VARCHAR(255);

ALTER TABLE transactions
    ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('web', 'telegram', 'mcp', 'import', 'system'));

-- Backfill. Server-derived rows (opening balances, settlement reimbursements,
-- investment cash legs) are 'system'; importer rows are 'import'. The import
-- update runs last so an imported row that is also an investment row is
-- 'import', the more specific provenance.
UPDATE transactions SET source = 'system'
    WHERE type IN ('opening_balance', 'reimbursement', 'investment');
UPDATE transactions SET source = 'import' WHERE import_ref IS NOT NULL;

-- pg_trgm may live in any schema: Supabase keeps extensions in "extensions",
-- which the migrating role's search_path may not include. Install it only if
-- missing (into "extensions" when that schema exists, as Supabase expects),
-- then name the operator class with its schema so the index never depends on
-- search_path.
DO $$
DECLARE
    trgm_schema text;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'extensions') THEN
            CREATE EXTENSION pg_trgm WITH SCHEMA extensions;
        ELSE
            CREATE EXTENSION pg_trgm;
        END IF;
    END IF;

    SELECT n.nspname INTO trgm_schema
    FROM pg_extension e
    JOIN pg_namespace n ON n.oid = e.extnamespace
    WHERE e.extname = 'pg_trgm';

    EXECUTE format(
        'CREATE INDEX idx_transactions_description_trgm ON transactions USING gin (description %I.gin_trgm_ops)',
        trgm_schema);
    EXECUTE format(
        'CREATE INDEX idx_transactions_notes_trgm ON transactions USING gin (notes %I.gin_trgm_ops)',
        trgm_schema);
END
$$;
