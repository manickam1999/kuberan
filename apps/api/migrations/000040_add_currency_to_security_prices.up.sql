-- Prices are stored in each security's own currency (plan 024, phase 2).
-- Every stored price is in its security's labelled currency today, because the
-- oracle converted to MYR and every security is labelled MYR.
ALTER TABLE security_prices ADD COLUMN currency VARCHAR(3);
UPDATE security_prices sp SET currency = UPPER(s.currency) FROM securities s WHERE s.id = sp.security_id;
ALTER TABLE security_prices ALTER COLUMN currency SET NOT NULL;
