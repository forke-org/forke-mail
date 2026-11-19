-- +goose Up
CREATE TABLE access_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'joined')),
    invite_token_hash text UNIQUE,
    invite_expires_at timestamptz,
    invites_sent int NOT NULL DEFAULT 0,
    workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    joined_at timestamptz
);
CREATE UNIQUE INDEX access_requests_email_key ON access_requests (lower(email));
CREATE INDEX access_requests_status_idx ON access_requests (status, created_at DESC);

ALTER TABLE users ADD COLUMN username text CHECK (username ~ '^[a-z0-9][a-z0-9_.-]{2,31}$');
CREATE UNIQUE INDEX users_username_key ON users (lower(username)) WHERE username IS NOT NULL;

-- +goose Down
DROP INDEX users_username_key;
ALTER TABLE users DROP COLUMN username;
DROP TABLE access_requests;
