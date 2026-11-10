ALTER TABLE telegram_links DROP COLUMN IF EXISTS remind_fd_maturity;
ALTER TABLE telegram_links DROP COLUMN IF EXISTS remind_loan_due;
ALTER TABLE telegram_links DROP COLUMN IF EXISTS remind_card_due;
DROP TABLE IF EXISTS reminders;
