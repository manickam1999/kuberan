ALTER TABLE transaction_rules
    ADD COLUMN match_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_matched_at TIMESTAMPTZ NULL;
