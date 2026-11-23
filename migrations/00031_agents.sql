-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
ALTER TABLE api_keys ADD COLUMN mode text NOT NULL DEFAULT 'live' CHECK (mode IN ('live', 'test'));
ALTER TABLE transactional_messages DROP CONSTRAINT transactional_messages_status_check;
ALTER TABLE transactional_messages ADD CONSTRAINT transactional_messages_status_check CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'test'));

CREATE TABLE oauth_clients (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    redirect_uris text[] NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_codes (
    code_hash text PRIMARY KEY,
    client_id text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    challenge text NOT NULL,
    expires_at timestamptz NOT NULL
);

CREATE TABLE oauth_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id text NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    access_hash text NOT NULL UNIQUE,
    refresh_hash text NOT NULL UNIQUE,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);
CREATE INDEX oauth_grants_user_idx ON oauth_grants (user_id);

CREATE TABLE agent_approvals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    requested_by text NOT NULL,
    tool text NOT NULL,
    args jsonb NOT NULL,
    summary text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'used')),
    decided_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX agent_approvals_workspace_idx ON agent_approvals (workspace_id, created_at DESC);

-- +goose Down
DROP TABLE agent_approvals;
DROP TABLE oauth_grants;
DROP TABLE oauth_codes;
DROP TABLE oauth_clients;
DELETE FROM transactional_messages WHERE status = 'test';
ALTER TABLE transactional_messages DROP CONSTRAINT transactional_messages_status_check;
ALTER TABLE transactional_messages ADD CONSTRAINT transactional_messages_status_check CHECK (status IN ('queued', 'sending', 'sent', 'failed'));
ALTER TABLE api_keys DROP COLUMN mode;
