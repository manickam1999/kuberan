-- The currency a user's aggregates are reported in (plan 024).
ALTER TABLE users ADD COLUMN base_currency VARCHAR(3) NOT NULL DEFAULT 'MYR';
