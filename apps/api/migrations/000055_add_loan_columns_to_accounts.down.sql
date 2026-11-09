ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_monthly_payment_nonneg;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_due_day_range;
ALTER TABLE accounts DROP COLUMN IF EXISTS due_day;
ALTER TABLE accounts DROP COLUMN IF EXISTS monthly_payment;
