-- The currency a snapshot is reported in, and whether holdings were skipped for lack of an FX rate (plan 024, phase 2).
ALTER TABLE portfolio_snapshots
    ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT 'MYR',
    ADD COLUMN fx_missing BOOLEAN NOT NULL DEFAULT false;
