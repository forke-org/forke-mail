-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
CREATE TABLE templates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    kind text NOT NULL DEFAULT 'transactional' CHECK (kind IN ('transactional', 'marketing')),
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 300),
    html text NOT NULL DEFAULT '',
    text text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX templates_workspace_name_key ON templates (workspace_id, lower(name));

-- +goose Down
DROP TABLE templates;
