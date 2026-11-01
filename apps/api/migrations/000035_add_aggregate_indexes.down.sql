CREATE INDEX IF NOT EXISTS idx_portfolio_snapshots_user_id ON portfolio_snapshots (user_id);
CREATE INDEX IF NOT EXISTS idx_security_prices_security_id ON security_prices (security_id);

DROP INDEX IF EXISTS idx_transactions_account_date;
DROP INDEX IF EXISTS idx_transactions_user_type_date;
