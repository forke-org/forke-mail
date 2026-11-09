-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION queue_contact_webhooks() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    event_name text;
    c contacts;
    reason text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        event_name := 'contact.deleted';
        c := OLD;
    ELSE
        IF NEW.status IS NOT DISTINCT FROM OLD.status
            AND NEW.paused_until IS NOT DISTINCT FROM OLD.paused_until
            AND NEW.topics_opted_out IS NOT DISTINCT FROM OLD.topics_opted_out THEN
            RETURN NEW;
        END IF;
        event_name := 'contact.updated';
        c := NEW;
    END IF;
    SELECT s.source INTO reason FROM suppressions s WHERE s.workspace_id = c.workspace_id AND lower(s.email) = lower(c.email);
    INSERT INTO webhook_deliveries (workspace_id, endpoint_id, event_id, payload)
    SELECT c.workspace_id, e.id, nextval(pg_get_serial_sequence('email_events', 'id')), jsonb_build_object(
        'id', gen_random_uuid()::text, 'version', '1', 'type', event_name, 'created_at', now(),
        'data', jsonb_strip_nulls(jsonb_build_object(
            'contact_id', c.id, 'email', c.email, 'status', c.status, 'status_changed_at', c.status_changed_at,
            'paused_until', c.paused_until, 'topics_opted_out', to_jsonb(c.topics_opted_out), 'suppression_source', reason)))::text
    FROM webhook_endpoints e
    WHERE e.workspace_id = c.workspace_id AND e.enabled AND event_name = ANY(e.events);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER contacts_customer_webhooks AFTER UPDATE OR DELETE ON contacts
FOR EACH ROW EXECUTE FUNCTION queue_contact_webhooks();

-- +goose Down
DROP TRIGGER contacts_customer_webhooks ON contacts;
DROP FUNCTION queue_contact_webhooks();
