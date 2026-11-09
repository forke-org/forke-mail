-- +goose Up
CREATE TABLE topics (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    key text NOT NULL CHECK (key ~ '^[a-z0-9][a-z0-9_-]{0,39}$'),
    label text NOT NULL CHECK (length(label) BETWEEN 1 AND 60),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, key)
);

ALTER TABLE contacts
    ADD COLUMN paused_until timestamptz,
    ADD COLUMN topics_opted_out text[] NOT NULL DEFAULT '{}',
    ADD COLUMN status_changed_at timestamptz NOT NULL DEFAULT now();

-- +goose StatementBegin
CREATE FUNCTION contacts_touch_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status IS DISTINCT FROM OLD.status THEN
        NEW.status_changed_at := now();
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER contacts_status_changed BEFORE UPDATE OF status ON contacts
FOR EACH ROW EXECUTE FUNCTION contacts_touch_status();

ALTER TABLE emails ADD COLUMN topic text CHECK (topic ~ '^[a-z0-9][a-z0-9_-]{0,39}$');

ALTER TABLE suppressions DROP CONSTRAINT suppressions_source_check;
ALTER TABLE suppressions ADD CONSTRAINT suppressions_source_check CHECK (source IN (
    'unsubscribe_link', 'ses_bounce', 'ses_complaint', 'manual', 'import', 'preference_center', 'dormancy'));

-- +goose Down
DELETE FROM suppressions WHERE source IN ('preference_center', 'dormancy');
ALTER TABLE suppressions DROP CONSTRAINT suppressions_source_check;
ALTER TABLE suppressions ADD CONSTRAINT suppressions_source_check CHECK (source IN ('unsubscribe_link', 'ses_bounce', 'ses_complaint', 'manual', 'import'));
ALTER TABLE emails DROP COLUMN topic;
DROP TRIGGER contacts_status_changed ON contacts;
DROP FUNCTION contacts_touch_status();
ALTER TABLE contacts DROP COLUMN status_changed_at, DROP COLUMN topics_opted_out, DROP COLUMN paused_until;
DROP TABLE topics;
