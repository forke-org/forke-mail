-- +goose Up
CREATE TABLE contacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email text NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    first_name text NOT NULL DEFAULT '' CHECK (length(first_name) <= 100),
    last_name text NOT NULL DEFAULT '' CHECK (length(last_name) <= 100),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'unsubscribed', 'bounced', 'complained', 'invalid')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX contacts_workspace_email_key ON contacts (workspace_id, lower(email));
CREATE INDEX contacts_workspace_status_idx ON contacts (workspace_id, status);
CREATE INDEX contacts_workspace_created_idx ON contacts (workspace_id, created_at DESC);

CREATE TABLE tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 50)
);
CREATE UNIQUE INDEX tags_workspace_name_key ON tags (workspace_id, lower(name));

CREATE TABLE contact_tags (
    contact_id uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (contact_id, tag_id)
);
CREATE INDEX contact_tags_tag_idx ON contact_tags (tag_id);

CREATE TABLE suppressions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email text NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    reason text NOT NULL CHECK (reason IN ('unsubscribed', 'bounced', 'complained', 'manual')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX suppressions_workspace_email_key ON suppressions (workspace_id, lower(email));

-- +goose Down
DROP TABLE suppressions;
DROP TABLE contact_tags;
DROP TABLE tags;
DROP TABLE contacts;
