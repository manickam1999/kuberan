-- Aggregate and list indexes (performance hardening).
--
-- Daily/monthly summaries, spending-by-category and top-expenses filter on
-- (user_id, type, date) over live rows.
CREATE INDEX IF NOT EXISTS idx_transactions_user_type_date
    ON transactions (user_id, type, date)
    WHERE deleted_at IS NULL;

-- Account transaction lists order by date DESC, created_at DESC per account.
CREATE INDEX IF NOT EXISTS idx_transactions_account_date
    ON transactions (account_id, date DESC, created_at DESC)
    WHERE deleted_at IS NULL;

-- Redundant single-column indexes: each is a leading-column prefix of a
-- unique constraint index on the same table.
--   idx_security_prices_security_id   ⊂ uq_security_prices_security_recorded (security_id, recorded_at)
--   idx_portfolio_snapshots_user_id   ⊂ uq_portfolio_snapshots_user_recorded (user_id, recorded_at)
DROP INDEX IF EXISTS idx_security_prices_security_id;
DROP INDEX IF EXISTS idx_portfolio_snapshots_user_id;
