-- Native trade amounts and base-currency cost basis (plan 024, phase 3a).
-- From now on an investment's amounts are in the security's currency; the _base
-- columns hold the same figures in the user's base currency.
ALTER TABLE investment_transactions
  ADD COLUMN fx_rate_to_base NUMERIC(20,10),                         -- security ccy -> user base, at trade date; NULL = legacy row
  ADD COLUMN base_amount BIGINT,                                     -- total_amount in base at that rate (frozen)
  ADD COLUMN realized_gain_loss_base BIGINT NOT NULL DEFAULT 0;
ALTER TABLE investments
  ADD COLUMN cost_basis_base BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN realized_gain_loss_base BIGINT NOT NULL DEFAULT 0;
-- Before this phase every amount was entered in the user's base currency (MYR in practice), so the
-- legacy base figures equal the stored ones.
UPDATE investments SET cost_basis_base = cost_basis, realized_gain_loss_base = realized_gain_loss;
UPDATE investment_transactions SET base_amount = total_amount, realized_gain_loss_base = realized_gain_loss
  WHERE base_amount IS NULL;
