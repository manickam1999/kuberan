-- Receivables: shared expenses, claims, loans & deposits (plan 021).
-- An expense keeps its full amount (the account balance reflects it), while
-- personal_amount holds the user's own share. NULL means the whole amount is theirs.
-- Reports read COALESCE(personal_amount, amount).
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS personal_amount BIGINT NULL;
ALTER TABLE transactions ADD CONSTRAINT transactions_personal_amount_check
    CHECK (personal_amount IS NULL OR (personal_amount >= 0 AND personal_amount <= amount));

-- Money owed to the user, one row per counterparty per expense.
CREATE TABLE IF NOT EXISTS receivables (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ,

    user_id             UUID NOT NULL REFERENCES users(id),
    transaction_id      UUID NOT NULL REFERENCES transactions(id),
    counterparty_name   VARCHAR(100) NOT NULL,
    counterparty_key    VARCHAR(100) NOT NULL,           -- normalized in Go (lowercased, whitespace collapsed)
    kind                VARCHAR(20) NOT NULL DEFAULT 'split'
        CHECK (kind IN ('split', 'on_behalf', 'claim', 'loan', 'deposit', 'refund')),
    amount              BIGINT NOT NULL CHECK (amount > 0),
    paid_amount         BIGINT NOT NULL DEFAULT 0 CHECK (paid_amount >= 0),
    written_off_amount  BIGINT NOT NULL DEFAULT 0 CHECK (written_off_amount >= 0),
    status              VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'partially_paid', 'settled', 'written_off')),
    CONSTRAINT receivables_settled_within_amount_check
        CHECK (paid_amount + written_off_amount <= amount)
);

CREATE INDEX IF NOT EXISTS idx_receivables_user_status
    ON receivables (user_id, status);
CREATE INDEX IF NOT EXISTS idx_receivables_user_counterparty_key
    ON receivables (user_id, counterparty_key);
CREATE INDEX IF NOT EXISTS idx_receivables_user_kind
    ON receivables (user_id, kind);
CREATE INDEX IF NOT EXISTS idx_receivables_transaction_id
    ON receivables (transaction_id);
CREATE INDEX IF NOT EXISTS idx_receivables_deleted_at
    ON receivables (deleted_at);

-- A repayment against a receivable. transaction_id is the reimbursement
-- transaction that credited account_id.
CREATE TABLE IF NOT EXISTS receivable_settlements (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    user_id         UUID NOT NULL REFERENCES users(id),
    receivable_id   UUID NOT NULL REFERENCES receivables(id),
    transaction_id  UUID NOT NULL REFERENCES transactions(id),
    account_id      UUID NOT NULL REFERENCES accounts(id),
    amount          BIGINT NOT NULL CHECK (amount > 0),
    date            TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_receivable_settlements_receivable_id
    ON receivable_settlements (receivable_id);
CREATE INDEX IF NOT EXISTS idx_receivable_settlements_transaction_id
    ON receivable_settlements (transaction_id);
CREATE INDEX IF NOT EXISTS idx_receivable_settlements_user_id
    ON receivable_settlements (user_id);
CREATE INDEX IF NOT EXISTS idx_receivable_settlements_deleted_at
    ON receivable_settlements (deleted_at);

-- No portfolio_snapshots change: outstanding receivables are excluded from net worth (plan 021, D4).
