-- Reminders (plan 023, 5.6): one row per upcoming occurrence of a card due
-- date, loan instalment or fixed-deposit maturity. The API keeps them in sync
-- with accounts.due_day and fixed_deposit_details.maturity_date; the Telegram
-- bot pulls the due ones and confirms each as sent.
CREATE TABLE IF NOT EXISTS reminders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('card_due', 'loan_due', 'fd_maturity')),
    ref_id UUID NOT NULL,
    due_on DATE NOT NULL,
    remind_at TIMESTAMPTZ NOT NULL,
    sent_at TIMESTAMPTZ,
    enabled BOOLEAN NOT NULL DEFAULT TRUE
);

-- One reminder per occurrence of a due date.
CREATE UNIQUE INDEX IF NOT EXISTS idx_reminders_occurrence ON reminders (kind, ref_id, due_on);
CREATE INDEX IF NOT EXISTS idx_reminders_user ON reminders (user_id);
CREATE INDEX IF NOT EXISTS idx_reminders_pending ON reminders (remind_at) WHERE sent_at IS NULL AND enabled;

-- Per-kind switches in Settings -> Telegram.
ALTER TABLE telegram_links ADD COLUMN IF NOT EXISTS remind_card_due BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE telegram_links ADD COLUMN IF NOT EXISTS remind_loan_due BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE telegram_links ADD COLUMN IF NOT EXISTS remind_fd_maturity BOOLEAN NOT NULL DEFAULT TRUE;
