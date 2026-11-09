DROP INDEX IF EXISTS idx_transactions_notes_trgm;
DROP INDEX IF EXISTS idx_transactions_description_trgm;

ALTER TABLE transactions
    DROP CONSTRAINT IF EXISTS transactions_source_check;

ALTER TABLE transactions
    DROP COLUMN IF EXISTS source_client_id,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS notes;
