-- +goose Up
CREATE TABLE platform_guard (
    id int PRIMARY KEY CHECK (id = 1),
    active boolean NOT NULL DEFAULT false,
    reason text NOT NULL DEFAULT '',
    detail jsonb NOT NULL DEFAULT '{}',
    activated_at timestamptz,
    cleared_at timestamptz,
    checked_at timestamptz
);
INSERT INTO platform_guard (id) VALUES (1);

-- +goose StatementBegin
CREATE FUNCTION workspace_held(ws uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN NOT COALESCE((SELECT active FROM platform_guard WHERE id = 1), false) THEN false
    ELSE EXISTS (
        SELECT 1 FROM workspaces w WHERE w.id = ws AND NOT w.platform AND (
            COALESCE(w.warmup_until > now(), false)
            OR (SELECT COUNT(*) FROM (
                SELECT 1 FROM email_recipients r WHERE r.workspace_id = ws AND r.sent_at IS NOT NULL
                UNION ALL
                SELECT 1 FROM transactional_messages t WHERE t.workspace_id = ws AND t.sent_at IS NOT NULL
                LIMIT 5000) proven) < 5000))
    END
$$;
-- +goose StatementEnd

ALTER TABLE workspace_alerts DROP CONSTRAINT workspace_alerts_kind_check;
ALTER TABLE workspace_alerts ADD CONSTRAINT workspace_alerts_kind_check CHECK (kind IN ('reputation_warning', 'sending_paused', 'platform_guard'));

-- +goose Down
DELETE FROM workspace_alerts WHERE kind = 'platform_guard';
ALTER TABLE workspace_alerts DROP CONSTRAINT workspace_alerts_kind_check;
ALTER TABLE workspace_alerts ADD CONSTRAINT workspace_alerts_kind_check CHECK (kind IN ('reputation_warning', 'sending_paused'));
DROP FUNCTION workspace_held(uuid);
DROP TABLE platform_guard;
