-- A budget's amounts are in its own currency. Existing budgets take their
-- owner's base currency, which is what their amounts were implicitly in.
ALTER TABLE budgets ADD COLUMN currency VARCHAR(3);
UPDATE budgets b SET currency = u.base_currency FROM users u WHERE u.id = b.user_id;
UPDATE budgets SET currency = 'MYR' WHERE currency IS NULL;
ALTER TABLE budgets ALTER COLUMN currency SET NOT NULL;
