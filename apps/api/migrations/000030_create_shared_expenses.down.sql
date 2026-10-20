DROP TABLE IF EXISTS receivable_settlements;
DROP TABLE IF EXISTS receivables;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_personal_amount_check;
ALTER TABLE transactions DROP COLUMN IF EXISTS personal_amount;
