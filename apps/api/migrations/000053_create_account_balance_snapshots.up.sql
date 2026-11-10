-- One balance per account per UTC day, written by the snapshot pipeline
-- (plan 023, 5.1). Balances are in the account's own currency.
CREATE TABLE IF NOT EXISTS account_balance_snapshots (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    day DATE NOT NULL,
    balance BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL,

    PRIMARY KEY (account_id, day)
);
CREATE INDEX idx_account_balance_snapshots_user_day ON account_balance_snapshots(user_id, day);
