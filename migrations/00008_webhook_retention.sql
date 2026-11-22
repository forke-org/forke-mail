-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
CREATE INDEX webhook_deliveries_expiry_idx ON webhook_deliveries(created_at) WHERE status='pending';
CREATE INDEX webhook_deliveries_retention_idx ON webhook_deliveries(finished_at) WHERE status IN ('delivered','failed','cancelled');

-- +goose Down
DROP INDEX webhook_deliveries_retention_idx;
DROP INDEX webhook_deliveries_expiry_idx;
