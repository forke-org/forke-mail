// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"context"
	"log/slog"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/transactional"

	"github.com/jackc/pgx/v5"
)

const (
	approvalEmailsPerHour = 10
	ApprovalTemplate      = "agent-approval"
)

type Mailer interface {
	Accept(ctx context.Context, who auth.Identity, key string, req transactional.Submission) (transactional.Acceptance, error)
}

func (h *Handler) notifyApproval(workspaceID, approvalID, requester, summary string) {
	if h.Mail == nil || h.MailWorkspace == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var recent int
	var project string
	if err := h.Pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM agent_approvals WHERE workspace_id = $1 AND created_at > now() - interval '1 hour'), name
		FROM workspaces WHERE id = $1`, workspaceID).Scan(&recent, &project); err != nil {
		slog.Warn("approval email", "err", err)
		return
	}
	if recent > approvalEmailsPerHour {
		return
	}
	rows, err := h.Pool.Query(ctx, `
		SELECT u.email FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1 AND m.role = 'super_admin' LIMIT 5`, workspaceID)
	if err != nil {
		slog.Warn("approval email", "err", err)
		return
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		slog.Warn("approval email", "err", err)
		return
	}
	link := h.BaseURL + "/approve/" + approvalID
	subject := "An AI agent is waiting for your approval in " + project
	for _, to := range owners {
		if _, err := h.Mail.Accept(ctx, auth.Identity{WorkspaceID: h.MailWorkspace}, "approval-"+approvalID+"-"+to, transactional.Submission{
			To: to, FromName: "Forke Mail", FromAddress: h.MailFrom, Subject: subject, Template: ApprovalTemplate,
			Data: map[string]string{"project": project, "requester": requester, "summary": summary, "review_link": link},
			Tags: map[string]string{"category": "agent_approval"},
		}); err != nil {
			slog.Warn("approval email", "err", err)
		}
	}
}
