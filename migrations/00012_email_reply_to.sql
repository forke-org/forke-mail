-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
ALTER TABLE emails ADD COLUMN reply_to text NOT NULL DEFAULT '' CHECK (length(reply_to) <= 254);

-- +goose Down
ALTER TABLE emails DROP COLUMN reply_to;
