-- +goose Up
CREATE TABLE campaigns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX campaigns_workspace_name_key ON campaigns (workspace_id, lower(name));
CREATE INDEX campaigns_workspace_created_idx ON campaigns (workspace_id, created_at DESC);

CREATE TABLE emails (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    campaign_id uuid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 300),
    from_name text NOT NULL DEFAULT '' CHECK (length(from_name) <= 100),
    from_address text NOT NULL CHECK (length(from_address) BETWEEN 3 AND 254),
    html text NOT NULL DEFAULT '',
    text text NOT NULL DEFAULT '',
    audience_all boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'scheduled', 'sending', 'paused', 'done')),
    pause_reason text CHECK (pause_reason IN ('manual', 'trial_quota_exhausted', 'daily_limit_reached')),
    scheduled_at timestamptz,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT emails_paused_reason_check CHECK ((status = 'paused') = (pause_reason IS NOT NULL))
);
CREATE INDEX emails_campaign_idx ON emails (campaign_id, created_at DESC);
CREATE INDEX emails_workspace_created_idx ON emails (workspace_id, created_at DESC);
CREATE INDEX emails_sending_idx ON emails (workspace_id) WHERE status = 'sending';

CREATE TABLE email_tags (
    email_id uuid NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (email_id, tag_id)
);

CREATE TABLE email_contacts (
    email_id uuid NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    PRIMARY KEY (email_id, contact_id)
);

CREATE TABLE email_recipients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_id uuid NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    contact_id uuid REFERENCES contacts (id) ON DELETE SET NULL,
    email text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'skipped')),
    ses_message_id text,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    sent_at timestamptz,
    UNIQUE (email_id, contact_id)
);
CREATE INDEX email_recipients_email_status_idx ON email_recipients (email_id, status);
CREATE INDEX email_recipients_claim_idx ON email_recipients (workspace_id, next_attempt_at) WHERE status = 'pending';
CREATE INDEX email_recipients_stuck_idx ON email_recipients (claimed_at) WHERE status = 'sending';

-- +goose Down
DROP TABLE email_recipients;
DROP TABLE email_contacts;
DROP TABLE email_tags;
DROP TABLE emails;
DROP TABLE campaigns;
