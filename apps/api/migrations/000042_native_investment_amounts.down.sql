ALTER TABLE investments
  DROP COLUMN realized_gain_loss_base,
  DROP COLUMN cost_basis_base;
ALTER TABLE investment_transactions
  DROP COLUMN realized_gain_loss_base,
  DROP COLUMN base_amount,
  DROP COLUMN fx_rate_to_base;
