-- +goose Up
CREATE TABLE global_bounces (
    email text PRIMARY KEY CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
    first_workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,
    bounce_type text NOT NULL DEFAULT '',
    bounces int NOT NULL DEFAULT 1,
    first_bounced_at timestamptz NOT NULL DEFAULT now(),
    last_bounced_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO global_bounces (email, first_workspace_id, bounce_type, first_bounced_at, last_bounced_at)
SELECT DISTINCT ON (lower(email)) lower(email), workspace_id, 'Permanent', created_at, created_at
FROM suppressions WHERE source = 'ses_bounce' ORDER BY lower(email), created_at
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE global_bounces;
