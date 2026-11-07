package emails

import (
	"context"
	"slices"

	"forkemail/internal/db"
	"forkemail/internal/httpx"
)

const (
	maxAudienceTags     = 50
	maxAudienceContacts = 10000
)

type Audience struct {
	All        bool     `json:"all"`
	TagIDs     []string `json:"tag_ids"`
	ContactIDs []string `json:"contact_ids"`
	Size       int      `json:"size"`
}

const audienceMatch = `c.workspace_id = $1 AND c.status = 'active'
	AND NOT EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = $1 AND lower(s.email) = lower(c.email))
	AND (
		EXISTS (SELECT 1 FROM emails e WHERE e.id = $2 AND e.audience_all)
		OR EXISTS (SELECT 1 FROM email_contacts ec WHERE ec.email_id = $2 AND ec.contact_id = c.id)
		OR EXISTS (SELECT 1 FROM email_tags et JOIN contact_tags ct ON ct.tag_id = et.tag_id WHERE et.email_id = $2 AND ct.contact_id = c.id)
	)`

func (a Audience) validate() string {
	switch {
	case len(a.TagIDs) > maxAudienceTags || !httpx.ValidIDs(a.TagIDs):
		return "invalid_tag_ids"
	case len(a.ContactIDs) > maxAudienceContacts || !httpx.ValidIDs(a.ContactIDs):
		return "invalid_contact_ids"
	}
	return ""
}

func saveAudience(ctx context.Context, q db.Querier, ws, emailID string, a Audience) error {
	tagIDs := unique(a.TagIDs)
	contactIDs := unique(a.ContactIDs)
	if _, err := q.Exec(ctx, `UPDATE emails SET audience_all = $3 WHERE id = $1 AND workspace_id = $2`, emailID, ws, a.All); err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `DELETE FROM email_tags WHERE email_id = $1`, emailID); err != nil {
		return err
	}
	tags, err := q.Exec(ctx, `
		INSERT INTO email_tags (email_id, tag_id)
		SELECT $1, t.id FROM tags t WHERE t.workspace_id = $2 AND t.id = ANY($3::uuid[])`, emailID, ws, tagIDs)
	if err != nil {
		return err
	}
	if tags.RowsAffected() != int64(len(tagIDs)) {
		return invalidError{"invalid_tag_ids"}
	}
	if _, err := q.Exec(ctx, `DELETE FROM email_contacts WHERE email_id = $1`, emailID); err != nil {
		return err
	}
	contacts, err := q.Exec(ctx, `
		INSERT INTO email_contacts (email_id, contact_id)
		SELECT $1, c.id FROM contacts c WHERE c.workspace_id = $2 AND c.id = ANY($3::uuid[])`, emailID, ws, contactIDs)
	if err != nil {
		return err
	}
	if contacts.RowsAffected() != int64(len(contactIDs)) {
		return invalidError{"invalid_contact_ids"}
	}
	return nil
}

func loadAudience(ctx context.Context, q db.Querier, ws, emailID string) (Audience, error) {
	var a Audience
	err := q.QueryRow(ctx, `
		SELECT e.audience_all,
			ARRAY(SELECT tag_id::text FROM email_tags WHERE email_id = $2 ORDER BY tag_id),
			ARRAY(SELECT contact_id::text FROM email_contacts WHERE email_id = $2 ORDER BY contact_id),
			(SELECT COUNT(*) FROM contacts c WHERE `+audienceMatch+`)
		FROM emails e WHERE e.id = $2 AND e.workspace_id = $1`, ws, emailID).Scan(&a.All, &a.TagIDs, &a.ContactIDs, &a.Size)
	return a, err
}

func Snapshot(ctx context.Context, q db.Querier, ws, emailID string) (int64, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO email_recipients (email_id, workspace_id, contact_id, email)
		SELECT $2, $1, c.id, c.email FROM contacts c WHERE `+audienceMatch+`
		ON CONFLICT (email_id, contact_id) DO NOTHING`, ws, emailID)
	return tag.RowsAffected(), err
}

func unique(ids []string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}
