// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	_ "embed"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

//go:embed approve.html
var approveHTML string

var approvePage = template.Must(template.New("approve").Parse(approveHTML))

type approvalView struct {
	ID, Summary, Tool, Project, RequestedBy, Status, Error, Notice string
	Expires                                                        time.Time
	Owner                                                          bool
}

func (h *Handler) approvalSession(r *http.Request) (auth.Identity, bool) {
	ck, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		return auth.Identity{}, false
	}
	id, err := h.Auth.IdentifySession(r.Context(), ck.Value)
	return id, err == nil
}

func (h *Handler) loadApproval(r *http.Request, userID string) (approvalView, string, error) {
	v := approvalView{ID: chi.URLParam(r, "id")}
	if !httpx.ValidID(v.ID) {
		return v, "", pgx.ErrNoRows
	}
	var ws, role string
	var expired bool
	err := h.Pool.QueryRow(r.Context(), `
		SELECT a.workspace_id, a.summary, a.tool, a.requested_by, a.status, a.expires_at, a.expires_at < now(), w.name, m.role
		FROM agent_approvals a JOIN workspaces w ON w.id = a.workspace_id
		JOIN workspace_members m ON m.workspace_id = a.workspace_id AND m.user_id = $2
		WHERE a.id = $1`, v.ID, userID).Scan(&ws, &v.Summary, &v.Tool, &v.RequestedBy, &v.Status, &v.Expires, &expired, &v.Project, &role)
	if err != nil {
		return v, "", err
	}
	if expired && v.Status == "pending" {
		v.Status = "expired"
	}
	v.Owner = role == auth.SuperAdmin
	return v, ws, nil
}

func (h *Handler) ShowApproval(w http.ResponseWriter, r *http.Request) {
	id, ok := h.approvalSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	v, _, err := h.loadApproval(r, id.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		renderApproval(w, http.StatusNotFound, approvalView{Error: "This approval request does not exist, or it belongs to a project you are not in."})
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	renderApproval(w, http.StatusOK, v)
}

func (h *Handler) DecideApproval(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if r.ParseForm() != nil || !httpx.SameOrigin(r) {
		renderApproval(w, http.StatusForbidden, approvalView{Error: "This decision did not come from the Forke Mail page."})
		return
	}
	id, ok := h.approvalSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	v, _, err := h.loadApproval(r, id.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		renderApproval(w, http.StatusNotFound, approvalView{Error: "This approval request does not exist."})
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !v.Owner || v.Status != "pending" {
		renderApproval(w, http.StatusConflict, v)
		return
	}
	status := "denied"
	if r.PostForm.Get("decision") == "approve" {
		status = "approved"
	}
	if _, err := h.Pool.Exec(r.Context(), `
		UPDATE agent_approvals SET status = $2, decided_by = $3 WHERE id = $1 AND status = 'pending' AND expires_at > now()`, v.ID, status, id.UserID); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	v.Status = status
	v.Notice = map[string]string{"approved": "Approved. Tell your AI agent to continue; it can now do this once.", "denied": "Declined. The agent cannot do this."}[status]
	renderApproval(w, http.StatusOK, v)
}

func renderApproval(w http.ResponseWriter, status int, v approvalView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = approvePage.Execute(w, v)
}
