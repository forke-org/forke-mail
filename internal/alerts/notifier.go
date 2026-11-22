package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/transactional"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Mailer interface {
	Accept(ctx context.Context, who auth.Identity, key string, req transactional.Submission) (transactional.Acceptance, error)
}

type Notifier struct {
	Pool        *pgxpool.Pool
	Mail        Mailer
	WorkspaceID string
	From        string
	AppURL      string
	Client      *http.Client
}

type Alert struct {
	ID          int64
	WorkspaceID string
	Kind        string
	Detail      map[string]any
	Workspace   string
	SlackURL    string
	Emails      bool
}

func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		for {
			done, err := n.Once(ctx)
			if err != nil {
				slog.Error("alerts", "err", err)
			}
			if done == 0 || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (n *Notifier) Once(ctx context.Context) (int, error) {
	tx, err := n.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.workspace_id, a.kind, a.detail, w.name, w.alert_slack_url, w.alert_emails
		FROM workspace_alerts a JOIN workspaces w ON w.id = a.workspace_id
		WHERE a.notified_at IS NULL ORDER BY a.id LIMIT 20 FOR UPDATE OF a SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	alerts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Alert, error) {
		var a Alert
		err := r.Scan(&a.ID, &a.WorkspaceID, &a.Kind, &a.Detail, &a.Workspace, &a.SlackURL, &a.Emails)
		return a, err
	})
	if err != nil || len(alerts) == 0 {
		return 0, err
	}
	ids := make([]int64, 0, len(alerts))
	for _, a := range alerts {
		n.deliver(ctx, tx, a)
		ids = append(ids, a.ID)
	}
	if _, err := tx.Exec(ctx, `UPDATE workspace_alerts SET notified_at = now() WHERE id = ANY($1)`, ids); err != nil {
		return 0, err
	}
	return len(alerts), tx.Commit(ctx)
}

func (n *Notifier) deliver(ctx context.Context, tx pgx.Tx, a Alert) {
	subject, lines := Message(a)
	if a.SlackURL != "" {
		if err := n.slack(ctx, a.SlackURL, "*"+subject+"*\n"+strings.Join(lines, "\n")+"\n"+n.AppURL); err != nil {
			slog.Warn("alert slack", "workspace_id", a.WorkspaceID, "err", err)
		}
	}
	if !a.Emails || n.Mail == nil || n.WorkspaceID == "" {
		return
	}
	rows, err := tx.Query(ctx, `SELECT email FROM users WHERE workspace_id = $1 ORDER BY created_at LIMIT 10`, a.WorkspaceID)
	if err != nil {
		slog.Warn("alert recipients", "workspace_id", a.WorkspaceID, "err", err)
		return
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		slog.Warn("alert recipients", "workspace_id", a.WorkspaceID, "err", err)
		return
	}
	body, text := render(subject, lines, n.AppURL)
	for _, to := range emails {
		_, err := n.Mail.Accept(ctx, auth.Identity{WorkspaceID: n.WorkspaceID}, fmt.Sprintf("forke-alert-%d-%s", a.ID, to), transactional.Submission{
			To: to, FromName: "Forke Mail", FromAddress: n.From, Subject: subject, HTML: body, Text: text,
			Tags: map[string]string{"category": "alert"},
		})
		if err != nil {
			slog.Warn("alert email", "workspace_id", a.WorkspaceID, "err", err)
		}
	}
}

func (n *Notifier) slack(ctx context.Context, url, text string) error {
	if !strings.HasPrefix(url, "https://hooks.slack.com/") {
		return fmt.Errorf("not a slack webhook")
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := n.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("slack status %d", res.StatusCode)
	}
	return nil
}

func number(d map[string]any, key string) float64 {
	v, _ := d[key].(float64)
	return v
}

func pct(v float64) string {
	s := fmt.Sprintf("%.2f", v*100)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + "%"
}

func Message(a Alert) (string, []string) {
	d := a.Detail
	sent := int(number(d, "sent"))
	bounce, complaint := number(d, "bounce_rate"), number(d, "complaint_rate")
	stats := fmt.Sprintf("Last 30 days: %d sent, %s bounced, %s marked as spam.", sent, pct(bounce), pct(complaint))
	if a.Kind == "platform_guard" {
		return "Forke Mail protection is on", []string{
			fmt.Sprintf("Across all projects in the last %v: %d sent, %s bounced, %s marked as spam.", d["window"], sent, pct(bounce), pct(complaint)),
			"Broadcasts from projects that are still warming up or have sent fewer than 5,000 emails are on hold. Transactional mail and established projects keep sending.",
			"Find the project causing it in the admin panel (Mail users, sort by bounces), suspend it if needed, then turn protection off from Status.",
		}
	}
	if a.Kind == "sending_paused" {
		return "Sending is paused on " + a.Workspace, []string{
			stats,
			fmt.Sprintf("Forke Mail paused sending because the %s passed the limit (bounces %s, spam reports %s).", label(d["reason"]), pct(number(d, "pause_bounce_rate")), pct(number(d, "pause_complaint_rate"))),
			"Remove addresses that bounce, stop sending to people who did not sign up, then reply to this email so we can resume sending.",
		}
	}
	return "Your " + label(d["reason"]) + " is getting high on " + a.Workspace, []string{
		stats,
		fmt.Sprintf("Nothing is paused yet. Sending pauses automatically at %s bounces or %s spam reports, to protect your domain.", pct(number(d, "pause_bounce_rate")), pct(number(d, "pause_complaint_rate"))),
		"Check the failed deliveries list, remove old or bought addresses, and make it easy to unsubscribe.",
	}
}

func label(reason any) string {
	switch reason {
	case "bounce_rate_high", "bounce_rate_exceeded":
		return "bounce rate"
	case "complaint_rate_high", "complaint_rate_exceeded":
		return "spam report rate"
	}
	return "sending reputation"
}

func render(subject string, lines []string, appURL string) (string, string) {
	var b strings.Builder
	b.WriteString(`<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#111;max-width:560px">`)
	b.WriteString(`<h2 style="font-size:18px;margin:0 0 12px">` + html.EscapeString(subject) + `</h2>`)
	for _, l := range lines {
		b.WriteString(`<p style="margin:0 0 12px">` + html.EscapeString(l) + `</p>`)
	}
	b.WriteString(`<p style="margin:20px 0 0"><a href="` + html.EscapeString(appURL) + `" style="color:#111">Open Forke Mail</a></p></div>`)
	return b.String(), subject + "\n\n" + strings.Join(lines, "\n\n") + "\n\n" + appURL + "\n"
}
