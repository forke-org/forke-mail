-- +goose Up
CREATE TABLE reengagement_settings (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    enabled boolean NOT NULL DEFAULT false,
    from_name text NOT NULL DEFAULT '' CHECK (length(from_name) <= 100),
    from_address text NOT NULL CHECK (length(from_address) BETWEEN 3 AND 254),
    reply_to text NOT NULL DEFAULT '' CHECK (length(reply_to) <= 254),
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 300),
    html text NOT NULL CHECK (length(html) BETWEEN 1 AND 262144),
    inactive_days integer NOT NULL DEFAULT 180 CHECK (inactive_days BETWEEN 30 AND 730),
    grace_days integer NOT NULL DEFAULT 30 CHECK (grace_days BETWEEN 7 AND 90),
    min_broadcasts integer NOT NULL DEFAULT 3 CHECK (min_broadcasts BETWEEN 1 AND 50),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE reengagement_settings;
