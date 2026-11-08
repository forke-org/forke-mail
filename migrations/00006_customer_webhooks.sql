-- +goose Up
ALTER TABLE webhook_endpoints
    ADD COLUMN enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN secret_version integer NOT NULL DEFAULT 1,
    ADD COLUMN next_dispatch_at timestamptz NOT NULL DEFAULT now();

CREATE TABLE webhook_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    endpoint_id uuid NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    event_id bigint NOT NULL,
    payload text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','delivered','failed','cancelled')),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_token uuid,
    claimed_at timestamptz,
    response_code integer,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    UNIQUE(endpoint_id,event_id)
);
CREATE INDEX webhook_deliveries_pending_idx ON webhook_deliveries(next_attempt_at) WHERE status='pending';
CREATE INDEX webhook_deliveries_workspace_idx ON webhook_deliveries(workspace_id,endpoint_id,created_at DESC);
CREATE INDEX webhook_deliveries_processing_idx ON webhook_deliveries(claimed_at) WHERE status='processing';

-- +goose StatementBegin
CREATE FUNCTION queue_customer_webhooks() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_name text;
BEGIN
    event_name := CASE WHEN NEW.type='sending_paused' THEN 'workspace.sending_paused' ELSE 'email.' || NEW.type END;
    INSERT INTO webhook_deliveries(workspace_id,endpoint_id,event_id,payload)
    SELECT NEW.workspace_id,e.id,NEW.id,jsonb_build_object(
        'id',NEW.id::text,'version','1','type',event_name,'created_at',NEW.created_at,
        'data',jsonb_build_object('email_id',NEW.email_id,'recipient_id',NEW.recipient_id))::text
    FROM webhook_endpoints e
    WHERE e.workspace_id=NEW.workspace_id AND e.enabled AND event_name=ANY(e.events)
    ON CONFLICT(endpoint_id,event_id) DO NOTHING;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER email_events_customer_webhooks AFTER INSERT ON email_events
FOR EACH ROW EXECUTE FUNCTION queue_customer_webhooks();

-- +goose Down
DROP TRIGGER email_events_customer_webhooks ON email_events;
DROP FUNCTION queue_customer_webhooks();
DROP TABLE webhook_deliveries;
ALTER TABLE webhook_endpoints DROP COLUMN next_dispatch_at, DROP COLUMN secret_version, DROP COLUMN enabled;
