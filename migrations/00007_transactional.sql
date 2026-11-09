-- +goose Up
CREATE TABLE transactional_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    api_key_id uuid REFERENCES api_keys (id) ON DELETE SET NULL,
    idempotency_key text CHECK (length(idempotency_key) BETWEEN 1 AND 256),
    request_hash text NOT NULL,
    fingerprint text NOT NULL,
    to_address text NOT NULL CHECK (length(to_address) BETWEEN 3 AND 254),
    from_name text NOT NULL DEFAULT '' CHECK (length(from_name) <= 100),
    from_address text NOT NULL CHECK (length(from_address) BETWEEN 3 AND 254),
    reply_to text CHECK (length(reply_to) BETWEEN 3 AND 254),
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 300),
    html text NOT NULL DEFAULT '',
    text text NOT NULL DEFAULT '',
    tags jsonb NOT NULL DEFAULT '{}',
    track_opens boolean NOT NULL DEFAULT false,
    track_clicks boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'sent', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    last_error text,
    ses_message_id text,
    quota_day date,
    quota_trial boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at timestamptz,
    delivered_at timestamptz,
    bounced_at timestamptz,
    bounce_type text,
    complained_at timestamptz,
    opened_at timestamptz,
    open_count integer NOT NULL DEFAULT 0,
    clicked_at timestamptz,
    click_count integer NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX transactional_messages_idempotency_key ON transactional_messages (workspace_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX transactional_messages_ses_message_id_key ON transactional_messages (ses_message_id) WHERE ses_message_id IS NOT NULL;
CREATE INDEX transactional_messages_workspace_created_idx ON transactional_messages (workspace_id, created_at DESC);
CREATE INDEX transactional_messages_fingerprint_idx ON transactional_messages (workspace_id, fingerprint, created_at DESC);
CREATE INDEX transactional_messages_due_idx ON transactional_messages (next_attempt_at) WHERE status = 'queued';
CREATE INDEX transactional_messages_stuck_idx ON transactional_messages (claimed_at) WHERE status = 'sending';
CREATE INDEX transactional_messages_workspace_sent_idx ON transactional_messages (workspace_id, sent_at) WHERE sent_at IS NOT NULL;

ALTER TABLE email_events ADD COLUMN message_id uuid REFERENCES transactional_messages (id) ON DELETE CASCADE;
CREATE INDEX email_events_message_idx ON email_events (message_id, id) WHERE message_id IS NOT NULL;
ALTER TABLE email_events DROP CONSTRAINT email_events_type_check;
ALTER TABLE email_events ADD CONSTRAINT email_events_type_check CHECK (type IN (
    'queued', 'sent', 'delivered', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'failed', 'rejected', 'delayed', 'sending_paused',
    'opened', 'clicked'));

ALTER TABLE message_logs
    ALTER COLUMN email_id DROP NOT NULL,
    ALTER COLUMN recipient_id DROP NOT NULL,
    ADD COLUMN message_id uuid REFERENCES transactional_messages (id) ON DELETE CASCADE,
    ADD CONSTRAINT message_logs_target_check CHECK ((recipient_id IS NULL) <> (message_id IS NULL));

-- +goose Down
DELETE FROM message_logs WHERE message_id IS NOT NULL;
ALTER TABLE message_logs
    DROP CONSTRAINT message_logs_target_check,
    DROP COLUMN message_id,
    ALTER COLUMN recipient_id SET NOT NULL,
    ALTER COLUMN email_id SET NOT NULL;
DELETE FROM email_events WHERE message_id IS NOT NULL OR type IN ('opened', 'clicked');
ALTER TABLE email_events DROP CONSTRAINT email_events_type_check;
ALTER TABLE email_events ADD CONSTRAINT email_events_type_check CHECK (type IN (
    'queued', 'sent', 'delivered', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'failed', 'rejected', 'delayed', 'sending_paused'));
DROP INDEX email_events_message_idx;
ALTER TABLE email_events DROP COLUMN message_id;
DROP TABLE transactional_messages;
