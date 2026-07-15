-- Budgets become recurring calendar-period caps (plan 016, D1).
-- Progress is always computed against the current month/year (time.Now()), so the
-- stored start_date/end_date columns never affected behavior. Drop them.
ALTER TABLE budgets DROP COLUMN IF EXISTS start_date;
ALTER TABLE budgets DROP COLUMN IF EXISTS end_date;

-- Constrain period to the two supported recurring windows.
ALTER TABLE budgets ADD CONSTRAINT budgets_period_check CHECK (period IN ('monthly', 'yearly'));

-- Speed up progress lookups keyed by (user, category).
CREATE INDEX IF NOT EXISTS idx_budgets_user_category ON budgets (user_id, category_id);

-- D4: at most one *active* budget per (user, category, period), ignoring soft-deleted rows.
CREATE UNIQUE INDEX IF NOT EXISTS uq_budgets_active_user_category_period
    ON budgets (user_id, category_id, period)
    WHERE deleted_at IS NULL AND is_active;
