-- Development database bootstrap (runs once, on an empty postgres volume).
--
-- The container's bootstrap superuser is `postgres` (POSTGRES_USER in
-- docker-compose.yml). The app connects as `kuberan`, an ordinary role that
-- owns its databases but is NOT a superuser, matching production (Supabase
-- never hands out superuser). Nothing in the app or migrations needs
-- superuser: pgcrypto is a trusted extension (a database owner may create
-- it), and it is pre-created below anyway.
--
-- Existing dev volumes created before this change keep their old layout
-- (kuberan as the bootstrap superuser); `docker compose down -v` to reset.

CREATE ROLE kuberan WITH LOGIN PASSWORD 'kuberan' NOSUPERUSER NOCREATEROLE CREATEDB;

CREATE DATABASE kuberan OWNER kuberan;

-- Dedicated database for Ory Hydra (OAuth 2.1 Authorization Server).
-- Hydra manages its own schema via `hydra migrate sql`; it is not touched by
-- golang-migrate. See plans/015-mcp-oauth-hydra.
CREATE DATABASE hydra OWNER kuberan;

\connect kuberan

-- Extensions used by the migrations (000001 creates pgcrypto itself; doing it
-- here as the bootstrap superuser keeps that a no-op).
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
