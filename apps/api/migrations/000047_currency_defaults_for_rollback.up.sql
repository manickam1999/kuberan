-- Compatibility defaults for older API images (plan 024 rollback).
-- Migrations 40 and 45 made these columns NOT NULL without a default. An API image
-- from before plan 024 does not know the columns and omits them on INSERT, which would
-- fail during the migrate -> new-image window or after an automatic image rollback.
-- The current code always sets currency explicitly, so this default is only ever used
-- by an old image. (Migrations 38-46 add no other NOT NULL column without a default.)
ALTER TABLE security_prices ALTER COLUMN currency SET DEFAULT 'MYR';
ALTER TABLE budgets ALTER COLUMN currency SET DEFAULT 'MYR';
