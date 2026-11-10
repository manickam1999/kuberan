-- Plan 023 5.7: every row the demo household creates is listed here, so
-- DELETE /demo removes exactly those rows and never touches real data.
CREATE TABLE demo_rows (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    table_name VARCHAR(64) NOT NULL,
    row_id     UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_demo_rows_user_table_row ON demo_rows (user_id, table_name, row_id);
