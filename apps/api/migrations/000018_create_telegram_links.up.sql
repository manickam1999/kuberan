CREATE TABLE IF NOT EXISTS telegram_links (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    user_id UUID NOT NULL,
    telegram_user_id BIGINT NOT NULL DEFAULT 0,
    telegram_username VARCHAR(255) DEFAULT '',
    telegram_first_name VARCHAR(255) DEFAULT '',
    link_code VARCHAR(6) DEFAULT '',
    link_code_expires_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    last_message_at TIMESTAMPTZ,
    message_count BIGINT NOT NULL DEFAULT 0,

    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_telegram_links_user_id ON telegram_links (user_id);
CREATE INDEX IF NOT EXISTS idx_telegram_links_telegram_user_id ON telegram_links (telegram_user_id);
CREATE INDEX IF NOT EXISTS idx_telegram_links_is_active ON telegram_links (is_active);
