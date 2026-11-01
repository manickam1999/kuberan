-- Opening balances were stored as income named "Initial balance", so they
-- showed up as income in the cash-flow Sankey, recent activity and totals.
-- They now have their own type. Reclassify only rows that are unmistakably
-- the one written by account creation: income, that exact description, no
-- category, created within 5 seconds of its account, and the account's first
-- transaction. A real income someone named "Initial balance" fails at least
-- one of these and stays income.
UPDATE transactions t
SET type = 'opening_balance'
FROM accounts a
WHERE t.account_id = a.id
  AND t.type = 'income'
  AND t.description = 'Initial balance'
  AND t.category_id IS NULL
  AND t.deleted_at IS NULL
  AND t.created_at BETWEEN a.created_at - INTERVAL '5 seconds'
                       AND a.created_at + INTERVAL '5 seconds'
  AND NOT EXISTS (
    SELECT 1 FROM transactions e
    WHERE e.account_id = t.account_id
      AND e.id <> t.id
      AND e.created_at < t.created_at
  );
