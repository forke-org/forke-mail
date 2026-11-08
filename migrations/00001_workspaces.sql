-- +goose Up
CREATE TABLE workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    plan text NOT NULL CHECK (plan IN ('paid', 'trial')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
    daily_limit integer NOT NULL DEFAULT 1000 CHECK (daily_limit >= 0),
    rate_limit_per_sec integer NOT NULL DEFAULT 1 CHECK (rate_limit_per_sec > 0),
    trial_send_limit integer NOT NULL DEFAULT 100 CHECK (trial_send_limit >= 0),
    trial_sent_count integer NOT NULL DEFAULT 0 CHECK (trial_sent_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT workspaces_trial_quota_check CHECK (plan <> 'trial' OR trial_sent_count BETWEEN 0 AND trial_send_limit)
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    email text NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
CREATE INDEX users_workspace_idx ON users (workspace_id);

CREATE TABLE sessions (
    token_hash text PRIMARY KEY CHECK (length(token_hash) = 64),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    scope text NOT NULL CHECK (scope IN ('full_access', 'sending_access')),
    prefix text NOT NULL CHECK (length(prefix) = 12),
    key_hash text NOT NULL CHECK (length(key_hash) = 64),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);
CREATE UNIQUE INDEX api_keys_key_hash_key ON api_keys (key_hash);
CREATE INDEX api_keys_workspace_idx ON api_keys (workspace_id, created_at DESC);

CREATE TABLE domains (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 253),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified')),
    dkim_tokens text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX domains_workspace_name_key ON domains (workspace_id, lower(name));

-- +goose Down
DROP TABLE domains;
DROP TABLE api_keys;
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE workspaces;
