-- +goose Up
ALTER TABLE users
    ALTER COLUMN password_hash DROP NOT NULL,
    ADD COLUMN forke_admin_id uuid,
    ADD COLUMN role text NOT NULL DEFAULT 'super_admin' CHECK (role IN ('super_admin', 'admin')),
    ADD CONSTRAINT users_credentials_check CHECK (password_hash IS NOT NULL OR forke_admin_id IS NOT NULL);
CREATE UNIQUE INDEX users_forke_admin_key ON users (forke_admin_id) WHERE forke_admin_id IS NOT NULL;

-- +goose Down
DELETE FROM users WHERE password_hash IS NULL;
DROP INDEX users_forke_admin_key;
ALTER TABLE users
    DROP CONSTRAINT users_credentials_check,
    DROP COLUMN role,
    DROP COLUMN forke_admin_id,
    ALTER COLUMN password_hash SET NOT NULL;
