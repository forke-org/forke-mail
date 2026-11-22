package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

type Runner struct {
	Pool *pgxpool.Pool
	Mail Mailer
}

type due struct {
	id, ws, email, firstName, lastName, contactStatus string
	step                                              int
	steps                                             []Step
	enabled, exited                                   bool
	contactData, eventData                            map[string]any
}

func (r Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		for {
			n, err := r.Once(ctx)
			if err != nil {
				slog.Error("workflows", "err", err)
			}
			if n == 0 || err != nil {
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

func (r Runner) Once(ctx context.Context) (int, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.workspace_id, c.email, c.first_name, c.last_name, c.status, r.step, w.steps, w.enabled AND ws.status = 'active', c.data, r.event_data,
			w.exit_event <> '' AND EXISTS (SELECT 1 FROM contact_events e WHERE e.contact_id = r.contact_id AND e.name = w.exit_event AND e.created_at > r.created_at)
		FROM workflow_runs r
		JOIN workflows w ON w.id = r.workflow_id
		JOIN contacts c ON c.id = r.contact_id
		JOIN workspaces ws ON ws.id = r.workspace_id
		WHERE r.status = 'running' AND r.next_run_at <= now()
		ORDER BY r.next_run_at LIMIT 50 FOR UPDATE OF r SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (due, error) {
		var d due
		var raw []byte
		if err := row.Scan(&d.id, &d.ws, &d.email, &d.firstName, &d.lastName, &d.contactStatus, &d.step, &raw, &d.enabled, &d.contactData, &d.eventData, &d.exited); err != nil {
			return d, err
		}
		return d, json.Unmarshal(raw, &d.steps)
	})
	if err != nil || len(list) == 0 {
		return 0, err
	}
	for _, d := range list {
		if err := r.advance(ctx, tx, d); err != nil {
			return 0, err
		}
	}
	return len(list), tx.Commit(ctx)
}

func (r Runner) finish(ctx context.Context, tx pgx.Tx, id, status, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE workflow_runs SET status = $2, last_error = NULLIF($3, ''), finished_at = now() WHERE id = $1`, id, status, reason)
	return err
}

func (r Runner) advance(ctx context.Context, tx pgx.Tx, d due) error {
	if !d.enabled {
		return r.finish(ctx, tx, d.id, "cancelled", "workflow_disabled")
	}
	if d.exited {
		return r.finish(ctx, tx, d.id, "done", "exit_event")
	}
	if d.step >= len(d.steps) {
		return r.finish(ctx, tx, d.id, "done", "")
	}
	s := d.steps[d.step]
	if s.Type == "wait" {
		_, err := tx.Exec(ctx, `UPDATE workflow_runs SET step = step + 1, next_run_at = now() + make_interval(mins => $2) WHERE id = $1`, d.id, s.Minutes)
		return err
	}
	if d.contactStatus != "active" {
		return r.finish(ctx, tx, d.id, "cancelled", "contact_"+d.contactStatus)
	}
	if r.Mail == nil {
		return r.finish(ctx, tx, d.id, "failed", "mailer_unavailable")
	}
	_, err := r.Mail.Accept(ctx, auth.Identity{WorkspaceID: d.ws}, fmt.Sprintf("wf-%s-%d", d.id, d.step), transactional.Submission{
		To: d.email, FromAddress: s.FromAddress, FromName: s.FromName, ReplyTo: s.ReplyTo, TemplateID: s.TemplateID, Data: variables(d),
		Tags: map[string]string{"source": "workflow"},
	})
	if err != nil {
		code := err.Error()
		if strings.HasPrefix(code, "template_") || strings.HasPrefix(code, "invalid_") || code == "recipient_suppressed" || code == "from_domain_not_verified" || code == "domain_not_verified" {
			return r.finish(ctx, tx, d.id, "failed", code)
		}
		_, err = tx.Exec(ctx, `UPDATE workflow_runs SET next_run_at = now() + interval '5 minutes', last_error = $2 WHERE id = $1`, d.id, code)
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workflow_runs SET step = step + 1, last_error = NULL, next_run_at = now() WHERE id = $1`, d.id)
	return err
}

func variables(d due) map[string]string {
	out := map[string]string{"email": d.email, "first_name": d.firstName, "last_name": d.lastName}
	for _, src := range []map[string]any{d.contactData, d.eventData} {
		for k, v := range src {
			if len(out) >= 100 {
				return out
			}
			switch x := v.(type) {
			case string:
				out[k] = x
			case nil:
			default:
				out[k] = fmt.Sprint(x)
			}
		}
	}
	return out
}
