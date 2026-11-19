-- +goose Up
ALTER TABLE workspaces ADD COLUMN platform boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX workspaces_platform_key ON workspaces (platform) WHERE platform;
UPDATE workspaces SET platform = true WHERE id = (
    SELECT u.workspace_id FROM users u WHERE u.forke_admin_id IS NOT NULL GROUP BY u.workspace_id ORDER BY COUNT(*) DESC LIMIT 1);

CREATE TABLE workspace_members (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('super_admin', 'admin')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX workspace_members_user_idx ON workspace_members (user_id);
INSERT INTO workspace_members (workspace_id, user_id, role, created_at) SELECT workspace_id, id, role, created_at FROM users;

-- +goose StatementBegin
CREATE FUNCTION sync_home_membership() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.workspace_id IS DISTINCT FROM NEW.workspace_id THEN
        INSERT INTO workspace_members (workspace_id, user_id, role) VALUES (NEW.workspace_id, NEW.id, NEW.role)
        ON CONFLICT (workspace_id, user_id) DO NOTHING;
        RETURN NEW;
    END IF;
    INSERT INTO workspace_members (workspace_id, user_id, role) VALUES (NEW.workspace_id, NEW.id, NEW.role)
    ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER users_home_membership AFTER INSERT OR UPDATE OF role, workspace_id ON users
FOR EACH ROW EXECUTE FUNCTION sync_home_membership();

ALTER TABLE sessions ADD COLUMN workspace_id uuid REFERENCES workspaces (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE sessions DROP COLUMN workspace_id;
DROP TRIGGER users_home_membership ON users;
DROP FUNCTION sync_home_membership();
DROP TABLE workspace_members;
DROP INDEX workspaces_platform_key;
ALTER TABLE workspaces DROP COLUMN platform;
