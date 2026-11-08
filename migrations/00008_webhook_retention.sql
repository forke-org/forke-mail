-- +goose Up
CREATE INDEX webhook_deliveries_expiry_idx ON webhook_deliveries(created_at) WHERE status='pending';
CREATE INDEX webhook_deliveries_retention_idx ON webhook_deliveries(finished_at) WHERE status IN ('delivered','failed','cancelled');

-- +goose Down
DROP INDEX webhook_deliveries_retention_idx;
DROP INDEX webhook_deliveries_expiry_idx;
