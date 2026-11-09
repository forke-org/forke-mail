-- +goose Up
ALTER TABLE emails
    ADD COLUMN track_opens boolean NOT NULL DEFAULT true,
    ADD COLUMN track_clicks boolean NOT NULL DEFAULT true;

ALTER TABLE email_recipients
    ADD COLUMN opened_at timestamptz,
    ADD COLUMN open_count integer NOT NULL DEFAULT 0 CHECK (open_count >= 0),
    ADD COLUMN clicked_at timestamptz,
    ADD COLUMN click_count integer NOT NULL DEFAULT 0 CHECK (click_count >= 0);

ALTER TABLE contacts
    ADD COLUMN last_engaged_at timestamptz,
    ADD COLUMN reengage_sent_at timestamptz;

-- +goose Down
ALTER TABLE contacts DROP COLUMN reengage_sent_at, DROP COLUMN last_engaged_at;
ALTER TABLE email_recipients DROP COLUMN click_count, DROP COLUMN clicked_at, DROP COLUMN open_count, DROP COLUMN opened_at;
ALTER TABLE emails DROP COLUMN track_clicks, DROP COLUMN track_opens;
