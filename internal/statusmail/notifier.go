// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package statusmail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/transactional"

	"github.com/jackc/pgx/v5"
)

const notifyWindow = 6 * time.Hour

type Incident struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Impact     string     `json:"impact"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

type Notifier struct {
	*Handler
	Source string
	Client *http.Client
}

func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		if err := n.Once(ctx); err != nil {
			slog.Warn("status notifier", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (n *Notifier) Once(ctx context.Context) error {
	incidents, err := n.fetch(ctx)
	if err != nil {
		return err
	}
	return n.Notify(ctx, incidents, time.Now())
}

func (n *Notifier) fetch(ctx context.Context) ([]Incident, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.Source, nil)
	if err != nil {
		return nil, err
	}
	client := n.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status source returned %d", res.StatusCode)
	}
	var doc struct {
		Incidents []Incident `json:"incidents"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&doc); err != nil {
		return nil, err
	}
	return doc.Incidents, nil
}

func (n *Notifier) Notify(ctx context.Context, incidents []Incident, now time.Time) error {
	if n.WorkspaceID == "" || n.Mail == nil {
		return nil
	}
	var opened, resolved []Incident
	for _, inc := range incidents {
		if inc.ID == "" || len(inc.ID) > 100 {
			continue
		}
		if inc.ResolvedAt != nil && now.Sub(*inc.ResolvedAt) < notifyWindow {
			claimedOpen, err := n.claim(ctx, inc.ID, "opened")
			if err != nil {
				return err
			}
			claimedResolved, err := n.claim(ctx, inc.ID, "resolved")
			if err != nil {
				return err
			}
			if claimedResolved && !claimedOpen {
				resolved = append(resolved, inc)
			}
			continue
		}
		if inc.ResolvedAt == nil && now.Sub(inc.StartedAt) < notifyWindow {
			claimed, err := n.claim(ctx, inc.ID, "opened")
			if err != nil {
				return err
			}
			if claimed {
				opened = append(opened, inc)
			}
		}
	}
	if len(opened) > 0 {
		if err := n.send(ctx, opened, "opened"); err != nil {
			return err
		}
	}
	if len(resolved) > 0 {
		return n.send(ctx, resolved, "resolved")
	}
	return nil
}

func (n *Notifier) claim(ctx context.Context, incidentID, kind string) (bool, error) {
	tag, err := n.Pool.Exec(ctx, `INSERT INTO status_notifications (incident_id, kind) VALUES ($1, $2) ON CONFLICT DO NOTHING`, incidentID, kind)
	return err == nil && tag.RowsAffected() == 1, err
}

func (n *Notifier) send(ctx context.Context, batch []Incident, kind string) error {
	rows, err := n.Pool.Query(ctx, `
		SELECT c.id, c.email FROM contacts c
		JOIN contact_tags ct ON ct.contact_id = c.id JOIN tags t ON t.id = ct.tag_id
		WHERE c.workspace_id = $1 AND t.workspace_id = $1 AND lower(t.name) = lower($2) AND c.status = 'active'
			AND NOT EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = $1 AND lower(s.email) = lower(c.email))`,
		n.WorkspaceID, SubscriberTag)
	if err != nil {
		return err
	}
	type recipient struct{ ID, Email string }
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[recipient])
	if err != nil {
		return err
	}
	ids := make([]string, len(batch))
	for i, inc := range batch {
		ids[i] = inc.ID
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, ",")))
	batchKey := hex.EncodeToString(sum[:8])
	data := incidentMessage(batch, kind)
	data["status_url"] = n.PageURL
	for _, r := range list {
		perRecipient := map[string]string{"stop_url": n.unsubscribeURL(r.Email)}
		for k, v := range data {
			perRecipient[k] = v
		}
		_, err := n.Mail.Accept(ctx, auth.Identity{WorkspaceID: n.WorkspaceID}, "status-"+kind+"-"+batchKey+"-"+r.ID, transactional.Submission{
			To: r.Email, FromName: "Forke Status", FromAddress: n.From, Subject: data["subject"], Template: IncidentTemplate, Data: perRecipient,
			Tags: map[string]string{"category": "status"},
		})
		if err != nil {
			slog.Warn("status email", "err", err)
		}
	}
	return nil
}

func names(batch []Incident) string {
	list := make([]string, len(batch))
	for i, inc := range batch {
		list[i] = inc.Name
	}
	if len(list) <= 2 {
		return strings.Join(list, " and ")
	}
	return strings.Join(list[:len(list)-1], ", ") + " and " + list[len(list)-1]
}

func incidentMessage(batch []Incident, kind string) map[string]string {
	first := batch[0]
	started := first.StartedAt.UTC().Format("15:04 UTC, 2 Jan")
	plural := len(batch) > 1
	if kind == "resolved" {
		mins := 1
		for _, inc := range batch {
			mins = max(mins, int(inc.ResolvedAt.Sub(inc.StartedAt).Minutes()))
		}
		subject := first.Name + " is back to normal"
		if plural {
			subject = fmt.Sprintf("%d Forke services are back to normal", len(batch))
		}
		return map[string]string{
			"subject": subject, "heading": "All clear.", "subheading": "Everything is working again.",
			"label": "Back to normal", "services": names(batch),
			"message":   fmt.Sprintf("%s had a problem from %s for about %d minutes. Everything is working normally again.", names(batch), started, mins),
			"next_step": "The status page has the full history.",
		}
	}
	down := false
	for _, inc := range batch {
		down = down || inc.Impact == "down"
	}
	what := "slower than usual"
	if down {
		what = "not working"
	}
	subject := first.Name + " is " + what
	verb := "is"
	if plural {
		subject = fmt.Sprintf("%d Forke services are %s", len(batch), what)
		verb = "are"
	}
	return map[string]string{
		"subject": subject, "heading": "Something's wrong.", "subheading": "We're on it.",
		"label": "Affected", "services": names(batch),
		"message":   fmt.Sprintf("Since %s, %s %s %s. We are looking into it.", started, names(batch), verb, what),
		"next_step": "We will email you again when it is fixed.",
	}
}
