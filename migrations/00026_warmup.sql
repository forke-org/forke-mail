-- +goose Up
ALTER TABLE workspaces ADD COLUMN warmup_until timestamptz;
CREATE INDEX emails_workspace_started_idx ON emails (workspace_id, started_at) WHERE started_at IS NOT NULL;

-- +goose Down
DROP INDEX emails_workspace_started_idx;
ALTER TABLE workspaces DROP COLUMN warmup_until;
