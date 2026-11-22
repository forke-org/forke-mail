-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
ALTER TABLE contacts ADD COLUMN data jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(data) = 'object');

CREATE TABLE contact_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$'),
    data jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX contact_events_workspace_name_idx ON contact_events (workspace_id, name, created_at DESC);
CREATE INDEX contact_events_contact_idx ON contact_events (contact_id, name, created_at DESC);

CREATE TABLE segments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    match text NOT NULL DEFAULT 'all' CHECK (match IN ('all', 'any')),
    rules jsonb NOT NULL DEFAULT '[]',
    size int NOT NULL DEFAULT 0,
    refreshed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX segments_workspace_name_key ON segments (workspace_id, lower(name));

CREATE TABLE segment_contacts (
    segment_id uuid NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    PRIMARY KEY (segment_id, contact_id)
);

CREATE TABLE email_segments (
    email_id uuid NOT NULL REFERENCES emails (id) ON DELETE CASCADE,
    segment_id uuid NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    PRIMARY KEY (email_id, segment_id)
);

CREATE TABLE workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    trigger_event text NOT NULL CHECK (trigger_event ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$'),
    steps jsonb NOT NULL DEFAULT '[]',
    enabled boolean NOT NULL DEFAULT false,
    allow_reentry boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workflows_trigger_idx ON workflows (workspace_id, trigger_event) WHERE enabled;

CREATE TABLE workflow_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES workflows (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    event_data jsonb NOT NULL DEFAULT '{}',
    step int NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done', 'failed', 'cancelled')),
    next_run_at timestamptz NOT NULL DEFAULT now(),
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX workflow_runs_due_idx ON workflow_runs (next_run_at) WHERE status = 'running';
CREATE INDEX workflow_runs_workflow_idx ON workflow_runs (workflow_id, created_at DESC);
CREATE INDEX workflow_runs_contact_idx ON workflow_runs (workflow_id, contact_id);

-- +goose Down
DROP TABLE workflow_runs;
DROP TABLE workflows;
DROP TABLE email_segments;
DROP TABLE segment_contacts;
DROP TABLE segments;
DROP TABLE contact_events;
ALTER TABLE contacts DROP COLUMN data;
