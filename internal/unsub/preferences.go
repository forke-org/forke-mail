package unsub

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"forkemail/internal/contacts"

	"github.com/jackc/pgx/v5"
)

const (
	pauseLength       = 30 * 24 * time.Hour
	resubscribeWindow = 30 * 24 * time.Hour
)

type topicChoice struct {
	Key        string
	Label      string
	Subscribed bool
}

type preferences struct {
	Email          string
	Topics         []topicChoice
	PausedUntil    *time.Time
	Unsubscribed   bool
	CanResubscribe bool
}

func loadPreferences(ctx context.Context, q pgx.Tx, t Target) (preferences, error) {
	var p preferences
	var status string
	var optedOut []string
	var reason *string
	var suppressedAt *time.Time
	err := q.QueryRow(ctx, `
		SELECT c.email, c.status, c.paused_until, c.topics_opted_out, s.reason, s.created_at
		FROM contacts c
		LEFT JOIN suppressions s ON s.workspace_id = c.workspace_id AND lower(s.email) = lower(c.email)
		WHERE c.id = $1 AND c.workspace_id = $2`, t.ContactID, t.WorkspaceID,
	).Scan(&p.Email, &status, &p.PausedUntil, &optedOut, &reason, &suppressedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, errContactGone
	}
	if err != nil {
		return p, err
	}
	if p.PausedUntil != nil && !p.PausedUntil.After(time.Now()) {
		p.PausedUntil = nil
	}
	p.Unsubscribed = status != "active" || reason != nil
	p.CanResubscribe = status == "unsubscribed" && reason != nil && *reason == "unsubscribed" &&
		suppressedAt != nil && time.Since(*suppressedAt) < resubscribeWindow
	topics, err := contacts.ListTopics(ctx, q, t.WorkspaceID)
	if err != nil {
		return p, err
	}
	for _, topic := range topics {
		p.Topics = append(p.Topics, topicChoice{Key: topic.Key, Label: topic.Label, Subscribed: !slices.Contains(optedOut, topic.Key)})
	}
	return p, nil
}

func saveTopics(ctx context.Context, tx pgx.Tx, t Target, subscribed []string) error {
	topics, err := contacts.ListTopics(ctx, tx, t.WorkspaceID)
	if err != nil {
		return err
	}
	optedOut := []string{}
	for _, topic := range topics {
		if !slices.Contains(subscribed, topic.Key) {
			optedOut = append(optedOut, topic.Key)
		}
	}
	_, err = tx.Exec(ctx, `UPDATE contacts SET topics_opted_out = $3 WHERE id = $1 AND workspace_id = $2`, t.ContactID, t.WorkspaceID, optedOut)
	return err
}

func setPause(ctx context.Context, tx pgx.Tx, t Target, until *time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE contacts SET paused_until = $3 WHERE id = $1 AND workspace_id = $2`, t.ContactID, t.WorkspaceID, until)
	return err
}

func resubscribe(ctx context.Context, tx pgx.Tx, t Target) error {
	p, err := loadPreferences(ctx, tx, t)
	if err != nil {
		return err
	}
	if !p.CanResubscribe {
		return errNotResubscribable
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM suppressions WHERE workspace_id = $1 AND lower(email) = lower($2) AND reason = 'unsubscribed'`,
		t.WorkspaceID, p.Email); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE contacts SET status = 'active', paused_until = NULL
		WHERE id = $1 AND workspace_id = $2 AND status = 'unsubscribed'`, t.ContactID, t.WorkspaceID)
	return err
}

func maskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return email
	}
	return local[:1] + strings.Repeat("•", min(len(local)-1, 6)) + "@" + domain
}
