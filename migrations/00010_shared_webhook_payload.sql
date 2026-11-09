-- +goose Up
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

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION queue_customer_webhooks() RETURNS trigger LANGUAGE plpgsql AS $$
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
