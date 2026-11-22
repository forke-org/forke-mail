-- +goose Up
ALTER TABLE workflows ADD COLUMN exit_event text NOT NULL DEFAULT '' CHECK (exit_event = '' OR exit_event ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$');

-- +goose Down
ALTER TABLE workflows DROP COLUMN exit_event;
