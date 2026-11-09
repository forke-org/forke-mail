-- +goose Up
ALTER TABLE emails ADD COLUMN reply_to text NOT NULL DEFAULT '' CHECK (length(reply_to) <= 254);

-- +goose Down
ALTER TABLE emails DROP COLUMN reply_to;
