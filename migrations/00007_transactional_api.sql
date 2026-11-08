-- +goose Up
CREATE TABLE transactional_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 128),
    request_hash text NOT NULL CHECK(length(request_hash)=64),
    mode text NOT NULL CHECK(mode IN ('engine','development_stub')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours',
    UNIQUE(workspace_id,idempotency_key)
);
CREATE INDEX transactional_requests_expiry_idx ON transactional_requests(expires_at);

-- +goose Down
DROP TABLE transactional_requests;
