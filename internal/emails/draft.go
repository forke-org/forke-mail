package emails

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Draft struct {
	CampaignID  string
	Subject     string
	FromName    string
	FromAddress string
	ReplyTo     string
	HTML        string
	ContactIDs  []string
}

func InsertDraft(ctx context.Context, tx pgx.Tx, ws string, d Draft) (string, error) {
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO emails (workspace_id, campaign_id, subject, from_name, from_address, reply_to, html)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		ws, d.CampaignID, d.Subject, d.FromName, d.FromAddress, d.ReplyTo, d.HTML).Scan(&id); err != nil {
		return "", err
	}
	return id, saveAudience(ctx, tx, ws, id, Audience{ContactIDs: d.ContactIDs})
}
