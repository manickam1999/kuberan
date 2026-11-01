DROP INDEX IF EXISTS idx_telegram_links_link_code;

-- 8-character codes do not fit the old column; clear any pending ones.
UPDATE telegram_links
SET link_code = '', link_code_expires_at = NULL
WHERE length(link_code) > 6;

ALTER TABLE telegram_links ALTER COLUMN link_code TYPE VARCHAR(6);
