-- Importer marker: "ibkr:<account>:<event key>". NULL for hand-entered rows.
-- A re-import finds and replaces its own rows through this column.
ALTER TABLE transactions ADD COLUMN import_ref VARCHAR(128);

CREATE INDEX idx_transactions_import_ref ON transactions (import_ref) WHERE import_ref IS NOT NULL;
