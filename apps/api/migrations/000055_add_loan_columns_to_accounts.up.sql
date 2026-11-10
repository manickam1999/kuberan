-- Loan accounts and recurring due days (plan 023, 5.3).
-- monthly_payment is in cents (account currency); due_day is the day of the
-- month (1-31) a card statement or loan instalment falls due.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS monthly_payment BIGINT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS due_day SMALLINT;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_due_day_range;
ALTER TABLE accounts ADD CONSTRAINT accounts_due_day_range CHECK (due_day IS NULL OR (due_day BETWEEN 1 AND 31));
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_monthly_payment_nonneg;
ALTER TABLE accounts ADD CONSTRAINT accounts_monthly_payment_nonneg CHECK (monthly_payment IS NULL OR monthly_payment >= 0);
