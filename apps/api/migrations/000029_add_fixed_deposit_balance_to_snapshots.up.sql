-- Add fixed_deposit_balance to portfolio_snapshots (plan 020).
-- Historical rows default to 0; no backfill needed (FD did not exist before).
ALTER TABLE portfolio_snapshots
    ADD COLUMN fixed_deposit_balance BIGINT NOT NULL DEFAULT 0;
