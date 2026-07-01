-- Grant necessary permissions to kuberan user
ALTER USER kuberan WITH SUPERUSER;

-- Create extensions if needed
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Set default privileges
ALTER DEFAULT PRIVILEGES IN SCHEMA public
GRANT ALL PRIVILEGES ON TABLES TO kuberan;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
GRANT ALL PRIVILEGES ON SEQUENCES TO kuberan;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
GRANT ALL PRIVILEGES ON FUNCTIONS TO kuberan;

-- Ensure the user owns the database
ALTER DATABASE kuberan OWNER TO kuberan;

-- Dedicated database for Ory Hydra (OAuth 2.1 Authorization Server).
-- Hydra manages its own schema via `hydra migrate sql`; it is not touched by
-- golang-migrate. See plans/015-mcp-oauth-hydra.
CREATE DATABASE hydra OWNER kuberan;
