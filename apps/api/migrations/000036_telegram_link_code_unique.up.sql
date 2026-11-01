-- Link codes grow from 6 hex characters to 8 base32 characters and must be
-- unique while pending ('' means no pending code).
ALTER TABLE telegram_links ALTER COLUMN link_code TYPE VARCHAR(16);

-- Drop pending codes that are expired, and any duplicates, so the unique index
-- can be built. Affected users simply generate a new code.
UPDATE telegram_links
SET link_code = '', link_code_expires_at = NULL
WHERE link_code <> ''
  AND (link_code_expires_at IS NULL OR link_code_expires_at < NOW());

UPDATE telegram_links t
SET link_code = '', link_code_expires_at = NULL
WHERE t.link_code <> ''
  AND EXISTS (
    SELECT 1 FROM telegram_links o
    WHERE o.link_code = t.link_code AND o.id <> t.id
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_links_link_code
    ON telegram_links (link_code)
    WHERE link_code <> '';
