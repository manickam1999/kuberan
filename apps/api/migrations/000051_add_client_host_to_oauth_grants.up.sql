-- Plan 023 item 1.6: the host an app's authorization code is sent to
-- (claude.ai, ...), shown under its name to tell apart apps with the same name.
ALTER TABLE oauth_client_grants
    ADD COLUMN client_host VARCHAR(255) NOT NULL DEFAULT '';
