-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
DROP TABLE agent_approvals;

-- +goose Down
CREATE TABLE agent_approvals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    requested_by text NOT NULL,
    tool text NOT NULL,
    args jsonb NOT NULL,
    summary text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'used')),
    decided_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX agent_approvals_workspace_idx ON agent_approvals (workspace_id, created_at DESC);
