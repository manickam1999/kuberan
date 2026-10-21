ALTER TABLE users ADD COLUMN default_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL;
