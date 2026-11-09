-- +goose Up
ALTER TABLE email_recipients ADD COLUMN claim_token uuid, ADD COLUMN provider_started_at timestamptz;
ALTER TABLE transactional_messages ADD COLUMN claim_token uuid, ADD COLUMN provider_started_at timestamptz;
UPDATE email_recipients SET provider_started_at=COALESCE(claimed_at,now()) WHERE status='sending';
UPDATE transactional_messages SET provider_started_at=COALESCE(claimed_at,now()) WHERE status='sending';
CREATE TABLE sending_control (
    id integer PRIMARY KEY CHECK(id=1),
    rate double precision NOT NULL CHECK(rate>0),
    burst integer NOT NULL CHECK(burst>0),
    tokens double precision NOT NULL,
    refilled_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    hold_until timestamptz NOT NULL DEFAULT '-infinity',
    urgent_streak integer NOT NULL DEFAULT 0,
    max_24h double precision NOT NULL DEFAULT 0,
    snapshot_sent double precision NOT NULL DEFAULT 0,
    snapshot_at timestamptz NOT NULL DEFAULT '-infinity'
);
CREATE TABLE sending_permits (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    state text NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','sending','used','released')),
    expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '30 seconds',
    finished_at timestamptz
);
CREATE INDEX sending_permits_active ON sending_permits(expires_at) WHERE state IN ('reserved','sending');
CREATE INDEX sending_permits_used ON sending_permits(finished_at) WHERE state='used';
CREATE TABLE sending_waiters (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    urgent boolean NOT NULL,
    ready_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '30 seconds'
);
CREATE TABLE workspace_send_gates (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    next_at timestamptz NOT NULL DEFAULT '-infinity'
);

-- +goose Down
DROP TABLE workspace_send_gates,sending_waiters,sending_permits,sending_control;
ALTER TABLE transactional_messages DROP COLUMN provider_started_at,DROP COLUMN claim_token;
ALTER TABLE email_recipients DROP COLUMN provider_started_at,DROP COLUMN claim_token;
