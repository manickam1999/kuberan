-- Plan 024 phase 4: a settlement paid into an account whose currency differs
-- from the receivable's currency stores the amount credited to the account, so
-- that deleting the settlement reverses the account by exactly that amount.
-- NULL means "same currency": the credit equals amount.
ALTER TABLE receivable_settlements
    ADD COLUMN account_amount BIGINT;

ALTER TABLE receivable_settlements
    ADD CONSTRAINT receivable_settlements_account_amount_check CHECK (account_amount > 0);
