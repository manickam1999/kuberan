-- Reverse plan 016 budget changes. For reversibility only; do NOT run against a
-- database that holds real recurring budgets (the re-added date columns are lossy).
DROP INDEX IF EXISTS uq_budgets_active_user_category_period;
DROP INDEX IF EXISTS idx_budgets_user_category;
ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_period_check;

ALTER TABLE budgets ADD COLUMN IF NOT EXISTS start_date TIMESTAMPTZ;
ALTER TABLE budgets ADD COLUMN IF NOT EXISTS end_date TIMESTAMPTZ;
