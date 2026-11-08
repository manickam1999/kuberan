-- Cross-currency transfers: the amount that arrives in the destination account,
-- in the destination's currency. NULL means the destination receives `amount`
-- (same-currency transfer, and every row that existed before this migration).
ALTER TABLE transactions
  ADD COLUMN to_amount BIGINT CHECK (to_amount IS NULL OR to_amount > 0);
