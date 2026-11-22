-- +goose Up
CREATE TABLE status_notifications (
    incident_id text NOT NULL CHECK (length(incident_id) BETWEEN 1 AND 100),
    kind text NOT NULL CHECK (kind IN ('opened', 'resolved')),
    sent_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, kind)
);

-- +goose Down
DROP TABLE status_notifications;
