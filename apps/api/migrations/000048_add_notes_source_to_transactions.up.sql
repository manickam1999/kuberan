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

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_transactions_description_trgm
    ON transactions USING gin (description gin_trgm_ops);
CREATE INDEX idx_transactions_notes_trgm
    ON transactions USING gin (notes gin_trgm_ops);
