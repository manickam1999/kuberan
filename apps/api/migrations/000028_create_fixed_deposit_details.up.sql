-- Fixed deposit details table (plan 020). One row per fixed_deposit account,
-- holding term/payout metadata. Rate lives on accounts.interest_rate.
CREATE TABLE IF NOT EXISTS fixed_deposit_details (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,

    account_id        UUID NOT NULL UNIQUE REFERENCES accounts(id),
    principal         BIGINT NOT NULL,
    start_date        DATE NOT NULL,
    maturity_date     DATE NOT NULL,
    payout_type       VARCHAR(16) NOT NULL CHECK (payout_type IN ('cumulative', 'periodic')),
    payout_frequency  VARCHAR(16) CHECK (payout_frequency IN ('monthly', 'quarterly', 'semiannual', 'annual')),
    compounding       VARCHAR(16) NOT NULL,
    linked_account_id UUID REFERENCES accounts(id),
    next_payout_date  DATE,
    maturity_value    BIGINT NOT NULL DEFAULT 0,
    status            VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'matured', 'closed'))
);

CREATE INDEX IF NOT EXISTS idx_fixed_deposit_details_account_id
    ON fixed_deposit_details (account_id);

CREATE INDEX IF NOT EXISTS idx_fixed_deposit_details_status_next_payout
    ON fixed_deposit_details (status, next_payout_date);

CREATE INDEX IF NOT EXISTS idx_fixed_deposit_details_deleted_at
    ON fixed_deposit_details (deleted_at);
