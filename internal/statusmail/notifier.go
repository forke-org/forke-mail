package statusmail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	for _, inc := range incidents {
		if inc.ID == "" || len(inc.ID) > 100 {
			continue
		}
		if now.Sub(inc.StartedAt) < notifyWindow {
			if err := n.send(ctx, inc, "opened"); err != nil {
				return err
			}
		}
		if inc.ResolvedAt != nil && now.Sub(*inc.ResolvedAt) < notifyWindow {
			if err := n.send(ctx, inc, "resolved"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *Notifier) send(ctx context.Context, inc Incident, kind string) error {
	tag, err := n.Pool.Exec(ctx, `INSERT INTO status_notifications (incident_id, kind) VALUES ($1, $2) ON CONFLICT DO NOTHING`, inc.ID, kind)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
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
	subject, lines := incidentMessage(inc, kind)
	for _, r := range list {
		body, text := render(subject, lines, n.PageURL, "Open status page", n.unsubscribeURL(r.Email))
		_, err := n.Mail.Accept(ctx, auth.Identity{WorkspaceID: n.WorkspaceID}, "status-"+kind+"-"+inc.ID+"-"+r.ID, transactional.Submission{
			To: r.Email, FromName: "Forke Status", FromAddress: n.From, Subject: subject, HTML: body, Text: text,
			Tags: map[string]string{"category": "status"},
		})
		if err != nil {
			slog.Warn("status email", "err", err)
		}
	}
	return nil
}

func incidentMessage(inc Incident, kind string) (string, []string) {
	started := inc.StartedAt.UTC().Format("15:04 UTC, 2 Jan")
	if kind == "resolved" {
		mins := int(inc.ResolvedAt.Sub(inc.StartedAt).Minutes())
		return inc.Name + " is back to normal", []string{
			fmt.Sprintf("%s had a problem from %s for about %d minutes. It is working normally again.", inc.Name, started, max(mins, 1)),
			"The status page has the full history.",
		}
	}
	what := "is slower than usual"
	if inc.Impact == "down" {
		what = "is not working"
	}
	return inc.Name + " " + what, []string{
		fmt.Sprintf("Since %s, %s %s. We are looking into it.", started, inc.Name, what),
		"We will email you again when it is fixed.",
	}
}
