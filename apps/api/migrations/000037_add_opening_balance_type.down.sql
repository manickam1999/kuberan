-- Every opening balance, whether reclassified by the up migration or created
-- since, was an income named "Initial balance" under the old scheme.
UPDATE transactions SET type = 'income' WHERE type = 'opening_balance';
