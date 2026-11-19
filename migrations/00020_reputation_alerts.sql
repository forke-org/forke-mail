-- +goose Up
ALTER TABLE workspaces
    ADD COLUMN reputation_warned_at timestamptz,
    ADD COLUMN alert_slack_url text NOT NULL DEFAULT '' CHECK (alert_slack_url = '' OR alert_slack_url LIKE 'https://hooks.slack.com/%'),
    ADD COLUMN alert_emails boolean NOT NULL DEFAULT true;

ALTER TABLE email_events DROP CONSTRAINT email_events_type_check;
ALTER TABLE email_events ADD CONSTRAINT email_events_type_check CHECK (type IN (
    'queued', 'sent', 'delivered', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'failed', 'rejected', 'delayed', 'sending_paused',
    'opened', 'clicked', 'reputation_warning'));

CREATE TABLE workspace_alerts (
    id bigserial PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('reputation_warning', 'sending_paused')),
    detail jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    notified_at timestamptz
);
CREATE INDEX workspace_alerts_pending_idx ON workspace_alerts (id) WHERE notified_at IS NULL;
CREATE INDEX workspace_alerts_workspace_idx ON workspace_alerts (workspace_id, id DESC);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION queue_customer_webhooks() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_name text;
DECLARE workspace_event boolean;
BEGIN
    workspace_event := NEW.type IN ('sending_paused', 'reputation_warning');
    event_name := CASE WHEN workspace_event THEN 'workspace.' || NEW.type ELSE 'email.' || NEW.type END;
    INSERT INTO webhook_deliveries(workspace_id,endpoint_id,event_id,payload)
    SELECT NEW.workspace_id,e.id,NEW.id,jsonb_build_object(
        'id',NEW.id::text,'version','1','type',event_name,'created_at',NEW.created_at,
        'data',CASE WHEN workspace_event
            THEN jsonb_build_object('message_type','workspace') || COALESCE(NEW.detail, '{}'::jsonb)
            ELSE jsonb_build_object('email_id',NEW.email_id,'recipient_id',NEW.recipient_id,'message_id',NEW.message_id,'message_type',CASE WHEN NEW.message_id IS NOT NULL THEN 'transactional' WHEN NEW.email_id IS NOT NULL THEN 'broadcast' ELSE 'workspace' END)
        END)::text
    FROM webhook_endpoints e
    WHERE e.workspace_id=NEW.workspace_id AND e.enabled AND event_name=ANY(e.events)
    ON CONFLICT(endpoint_id,event_id) DO NOTHING;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION queue_customer_webhooks() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_name text;
BEGIN
    event_name := CASE WHEN NEW.type='sending_paused' THEN 'workspace.sending_paused' ELSE 'email.' || NEW.type END;
    INSERT INTO webhook_deliveries(workspace_id,endpoint_id,event_id,payload)
    SELECT NEW.workspace_id,e.id,NEW.id,jsonb_build_object(
        'id',NEW.id::text,'version','1','type',event_name,'created_at',NEW.created_at,
        'data',jsonb_build_object('email_id',NEW.email_id,'recipient_id',NEW.recipient_id,'message_id',NEW.message_id,'message_type',CASE WHEN NEW.message_id IS NOT NULL THEN 'transactional' WHEN NEW.email_id IS NOT NULL THEN 'broadcast' ELSE 'workspace' END))::text
    FROM webhook_endpoints e
    WHERE e.workspace_id=NEW.workspace_id AND e.enabled AND event_name=ANY(e.events)
    ON CONFLICT(endpoint_id,event_id) DO NOTHING;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
DROP TABLE workspace_alerts;
DELETE FROM email_events WHERE type = 'reputation_warning';
ALTER TABLE email_events DROP CONSTRAINT email_events_type_check;
ALTER TABLE email_events ADD CONSTRAINT email_events_type_check CHECK (type IN (
    'queued', 'sent', 'delivered', 'bounced', 'soft_bounced', 'complained', 'unsubscribed', 'failed', 'rejected', 'delayed', 'sending_paused',
    'opened', 'clicked'));
ALTER TABLE workspaces DROP COLUMN alert_emails, DROP COLUMN alert_slack_url, DROP COLUMN reputation_warned_at;
