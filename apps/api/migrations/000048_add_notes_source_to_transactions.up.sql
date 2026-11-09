-- Plan 023 items 1.3/1.4: free-text notes, the origin of each row, and
-- trigram search over the description and notes.
ALTER TABLE transactions
    ADD COLUMN notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'web',
    ADD COLUMN source_client_id VARCHAR(255);

ALTER TABLE transactions
    ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('web', 'telegram', 'mcp', 'rule', 'import', 'system'));

-- Rows written before this migration by an importer or by account creation.
UPDATE transactions SET source = 'import' WHERE import_ref IS NOT NULL;
UPDATE transactions SET source = 'system' WHERE type = 'opening_balance';

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_transactions_description_trgm
    ON transactions USING gin (description gin_trgm_ops);
CREATE INDEX idx_transactions_notes_trgm
    ON transactions USING gin (notes gin_trgm_ops);
