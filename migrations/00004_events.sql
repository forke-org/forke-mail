-- +goose Up
CREATE TABLE message_logs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email_id uuid NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
    recipient_id uuid NOT NULL REFERENCES email_recipients (id) ON DELETE CASCADE,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX message_logs_email_idx ON message_logs (workspace_id, email_id, id);

CREATE TABLE email_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email_id uuid REFERENCES emails (id) ON DELETE CASCADE,
    recipient_id uuid REFERENCES email_recipients (id) ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('queued', 'sent', 'delivered', 'bounced', 'complained', 'unsubscribed', 'failed')),
    detail jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_events_workspace_idx ON email_events (workspace_id, id DESC);
CREATE INDEX email_events_workspace_type_idx ON email_events (workspace_id, type, id DESC);
CREATE INDEX email_events_email_type_idx ON email_events (email_id, type);

CREATE TABLE usage_daily (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    date date NOT NULL,
    sent_count integer NOT NULL DEFAULT 0 CHECK (sent_count >= 0),
    PRIMARY KEY (workspace_id, date)
);

CREATE TABLE webhook_endpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    url text NOT NULL CHECK (url LIKE 'https://%'),
    secret text NOT NULL CHECK (length(secret) >= 32),
    events text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_endpoints_workspace_idx ON webhook_endpoints (workspace_id);

-- +goose Down
DROP TABLE webhook_endpoints;
DROP TABLE usage_daily;
DROP TABLE email_events;
DROP TABLE message_logs;
