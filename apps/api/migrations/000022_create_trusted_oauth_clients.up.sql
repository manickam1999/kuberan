CREATE TABLE IF NOT EXISTS trusted_oauth_clients (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    client_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trusted_oauth_clients_client_id
    ON trusted_oauth_clients (client_id)
    WHERE deleted_at IS NULL;
