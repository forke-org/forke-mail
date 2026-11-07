-- +goose Up
ALTER TABLE domains
    ADD COLUMN kind text NOT NULL DEFAULT 'domain' CHECK (kind IN ('domain', 'email')),
    ADD COLUMN ses_status text,
    ADD COLUMN verified_at timestamptz,
    ADD COLUMN last_checked_at timestamptz;
ALTER TABLE domains DROP CONSTRAINT domains_status_check;
ALTER TABLE domains ADD CONSTRAINT domains_status_check CHECK (status IN ('pending', 'verified', 'failed'));
UPDATE domains SET verified_at = created_at WHERE status = 'verified';
DROP INDEX domains_workspace_name_key;
CREATE UNIQUE INDEX domains_name_key ON domains (lower(name));
CREATE INDEX domains_workspace_idx ON domains (workspace_id);
CREATE INDEX domains_pending_idx ON domains (last_checked_at) WHERE status = 'pending';

ALTER TABLE workspaces
    ADD COLUMN sending_paused boolean NOT NULL DEFAULT false,
    ADD COLUMN pause_reason text CHECK (pause_reason IN ('bounce_rate_exceeded', 'complaint_rate_exceeded')),
    ADD CONSTRAINT workspaces_sending_paused_check CHECK (sending_paused = (pause_reason IS NOT NULL));

ALTER TABLE emails DROP CONSTRAINT emails_pause_reason_check;
ALTER TABLE emails ADD CONSTRAINT emails_pause_reason_check CHECK (pause_reason IN (
    'manual', 'trial_quota_exhausted', 'daily_limit_reached', 'ses_sending_paused', 'identity_not_verified', 'reputation'));

ALTER TABLE email_recipients
    ADD COLUMN delivered_at timestamptz,
    ADD COLUMN bounced_at timestamptz,
    ADD COLUMN bounce_type text,
    ADD COLUMN complained_at timestamptz,
    ADD COLUMN unsubscribed_at timestamptz;
CREATE UNIQUE INDEX email_recipients_ses_message_id_key ON email_recipients (ses_message_id) WHERE ses_message_id IS NOT NULL;
CREATE INDEX email_recipients_workspace_sent_idx ON email_recipients (workspace_id, sent_at) WHERE sent_at IS NOT NULL;

ALTER TABLE email_events ADD COLUMN ses_message_id text;
CREATE INDEX email_events_ses_message_id_idx ON email_events (ses_message_id) WHERE ses_message_id IS NOT NULL;
ALTER TABLE email_events DROP CONSTRAINT email_events_type_check;
ALTER TABLE email_events ADD CONSTRAINT email_events_type_check CHECK (type IN (
    'queued', 'sent', 'delivered', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'failed', 'rejected', 'delayed', 'sending_paused'));

ALTER TABLE suppressions
    ADD COLUMN source text NOT NULL DEFAULT 'manual',
    ADD COLUMN detail jsonb NOT NULL DEFAULT '{}';
UPDATE suppressions SET source = CASE reason
    WHEN 'unsubscribed' THEN 'unsubscribe_link'
    WHEN 'bounced' THEN 'ses_bounce'
    WHEN 'complained' THEN 'ses_complaint'
    ELSE 'manual' END;
ALTER TABLE suppressions ADD CONSTRAINT suppressions_source_check CHECK (source IN ('unsubscribe_link', 'ses_bounce', 'ses_complaint', 'manual', 'import'));
CREATE INDEX suppressions_workspace_created_idx ON suppressions (workspace_id, created_at DESC);

CREATE TABLE processed_sns_messages (
    sns_message_id text PRIMARY KEY CHECK (length(sns_message_id) BETWEEN 1 AND 100),
    received_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE processed_sns_messages;
DROP INDEX suppressions_workspace_created_idx;
ALTER TABLE suppressions DROP COLUMN detail, DROP COLUMN source;
ALTER TABLE email_events DROP CONSTRAINT email_events_type_check;
DELETE FROM email_events WHERE type IN ('soft_bounced', 'rejected', 'delayed', 'sending_paused');
ALTER TABLE email_events ADD CONSTRAINT email_events_type_check CHECK (type IN (
    'queued', 'sent', 'delivered', 'bounced', 'complained', 'unsubscribed', 'failed'));
ALTER TABLE email_events DROP COLUMN ses_message_id;
DROP INDEX email_recipients_workspace_sent_idx;
DROP INDEX email_recipients_ses_message_id_key;
ALTER TABLE email_recipients
    DROP COLUMN unsubscribed_at, DROP COLUMN complained_at, DROP COLUMN bounce_type,
    DROP COLUMN bounced_at, DROP COLUMN delivered_at;
UPDATE emails SET pause_reason = 'manual' WHERE pause_reason IN ('ses_sending_paused', 'identity_not_verified', 'reputation');
ALTER TABLE emails DROP CONSTRAINT emails_pause_reason_check;
ALTER TABLE emails ADD CONSTRAINT emails_pause_reason_check CHECK (pause_reason IN ('manual', 'trial_quota_exhausted', 'daily_limit_reached'));
ALTER TABLE workspaces DROP CONSTRAINT workspaces_sending_paused_check, DROP COLUMN pause_reason, DROP COLUMN sending_paused;
DROP INDEX domains_pending_idx;
DROP INDEX domains_workspace_idx;
DROP INDEX domains_name_key;
CREATE UNIQUE INDEX domains_workspace_name_key ON domains (workspace_id, lower(name));
DELETE FROM domains WHERE status = 'failed';
ALTER TABLE domains DROP CONSTRAINT domains_status_check;
ALTER TABLE domains ADD CONSTRAINT domains_status_check CHECK (status IN ('pending', 'verified'));
ALTER TABLE domains DROP COLUMN last_checked_at, DROP COLUMN verified_at, DROP COLUMN ses_status, DROP COLUMN kind;
