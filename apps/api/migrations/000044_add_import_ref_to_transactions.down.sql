DROP INDEX IF EXISTS idx_transactions_import_ref;
ALTER TABLE transactions DROP COLUMN IF EXISTS import_ref;
