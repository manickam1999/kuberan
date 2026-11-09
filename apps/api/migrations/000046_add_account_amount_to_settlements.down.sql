ALTER TABLE receivable_settlements
    DROP CONSTRAINT IF EXISTS receivable_settlements_account_amount_check;

ALTER TABLE receivable_settlements
    DROP COLUMN IF EXISTS account_amount;
