-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
CREATE INDEX email_recipients_sent_idx ON email_recipients (sent_at) WHERE sent_at IS NOT NULL;
CREATE INDEX transactional_messages_sent_idx ON transactional_messages (sent_at) WHERE sent_at IS NOT NULL;

-- +goose Down
DROP INDEX transactional_messages_sent_idx;
DROP INDEX email_recipients_sent_idx;
