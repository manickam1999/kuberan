-- Receipt attachments for transactions (plan 017). Bytes live in MinIO/S3; only
-- metadata + an opaque storage_key are kept here.
CREATE TABLE IF NOT EXISTS transaction_attachments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,
    user_id         UUID NOT NULL REFERENCES users(id),
    transaction_id  UUID NOT NULL REFERENCES transactions(id),
    storage_key     VARCHAR(512) NOT NULL,
    file_name       VARCHAR(255) NOT NULL DEFAULT '',
    content_type    VARCHAR(128) NOT NULL,
    byte_size       BIGINT NOT NULL,
    checksum_sha256 CHAR(64) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tx_attachments_transaction_id ON transaction_attachments (transaction_id);
CREATE INDEX IF NOT EXISTS idx_tx_attachments_user_id       ON transaction_attachments (user_id);
CREATE INDEX IF NOT EXISTS idx_tx_attachments_deleted_at    ON transaction_attachments (deleted_at);
