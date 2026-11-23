// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type call struct {
	*session
	tool      *tool
	args      map[string]any
	confirmed bool
	approved  bool
}

func (s *session) principal() string {
	if s.id.KeyID != "" {
		return "key:" + s.id.KeyID
	}
	return "user:" + s.id.UserID + ":" + s.id.WorkspaceID
}

func (s *session) requester() string {
	if s.id.KeyID != "" {
		return "API key " + s.id.KeyPrefix + "…"
	}
	return "An AI app signed in as you"
}

func text(s string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": s}}, "isError": isError}
}

func (s *session) callTool(t *tool, args map[string]any) map[string]any {
	c := &call{session: s, tool: t, args: args}
	action := canonical(t.Name, args)
	conf := confirmer{s.h.Secret}
	if tok, _ := args["confirm_token"].(string); tok != "" {
		if !conf.valid(s.principal(), action, tok, time.Now()) {
			return text("That confirm_token is not valid for these exact arguments or has expired. Call the tool again without confirm_token to get a new one, and ask the user again.", true)
		}
		c.confirmed = true
	}
	if approval, _ := args["approval_id"].(string); approval != "" {
		msg, ok := c.redeem(approval)
		if !ok {
			return text(msg, true)
		}
		c.approved = true
	}
	out, err := t.Run(c, args)
	var g *gate
	if errors.As(err, &g) {
		switch g.risk {
		case Confirm:
			return text(fmt.Sprintf("CONFIRMATION NEEDED: %s\n\nShow this to the user and ask them to confirm. Only if they clearly say yes, call %s again with exactly the same arguments plus \"confirm_token\": %q. The token expires in 10 minutes.", g.summary, t.Name, conf.token(s.principal(), action, time.Now())), false)
		case Approve:
			return c.requestApproval(g.summary)
		}
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return text(explain(ae), true)
	}
	if err != nil {
		return text(err.Error(), true)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if len(b) > 60000 {
		b = append(b[:60000], []byte("\n… (truncated, use limit/offset or filters)")...)
	}
	return text(string(b), false)
}

func (c *call) do(method, path string, body any, summary string) (json.RawMessage, error) {
	return c.doWith(method, path, body, summary, nil)
}

func (c *call) doWith(method, path string, body any, summary string, headers map[string]string) (json.RawMessage, error) {
	switch risk := classify(method, path); {
	case risk == Blocked:
		return nil, errors.New("this action is not available to AI agents for safety. The user can do it in the Forke Mail dashboard")
	case risk == Approve && !c.approved:
		return nil, &gate{Approve, summary}
	case risk == Confirm && !c.confirmed && !c.approved && !(c.id.Test && testOnly(method, path)):
		return nil, &gate{Confirm, summary}
	}
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	ctx := context.WithValue(auth.WithIdentity(c.ctx, c.id), chi.RouteCtxKey, (*chi.Context)(nil))
	req, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = "mcp"
	rec := &capture{header: http.Header{}, status: http.StatusOK}
	c.h.API.ServeHTTP(rec, req)
	if rec.status >= 400 {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rec.body.Bytes(), &e)
		if e.Error == "" {
			e.Error = e.Message
		}
		return nil, &apiError{status: rec.status, code: e.Error, body: rec.body.String()}
	}
	if rec.body.Len() == 0 {
		return json.RawMessage(`{"ok":true}`), nil
	}
	return json.RawMessage(rec.body.Bytes()), nil
}

func testOnly(method, path string) bool {
	return method == http.MethodPost && (path == "/api/transactional" || path == "/api/forke-mail/emails" || path == "/api/emails")
}

type capture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capture) Header() http.Header         { return c.header }
func (c *capture) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *capture) WriteHeader(status int)      { c.status = status }

var hints = map[string]string{
	"sender_not_verified":                 "The from address is not on a verified domain. Use add_domain, give the user the DNS records, then check_domain until it is verified. With a test key any from address works.",
	"invalid_email_request":               "The email is missing something or has an invalid field: check to, from, subject and that html or text is set.",
	"template_not_found":                  "No template with that name or ID. Use list_templates.",
	"test_key_cannot_launch_broadcasts":   "Test keys cannot launch broadcasts. Use a live key or sign in with OAuth.",
	"trial_cannot_create_full_access_key": "Trial projects can only create sending keys. Use scope sending_access.",
	"forbidden":                           "Your role or key does not allow this. Ask the project owner, or use a key with full access.",
	"not_found":                           "Nothing with that ID in this project. List first to get the right ID.",
	"no_eligible_recipients":              "The broadcast audience is empty after removing unsubscribed and suppressed contacts.",
	"identity_taken":                      "That domain is already used by another Forke Mail project.",
	"rate_limited":                        "Too many requests. Wait a few seconds and try again.",
}

func explain(e *apiError) string {
	msg := fmt.Sprintf("Forke Mail returned %d %s.", e.status, e.code)
	if h, ok := hints[e.code]; ok {
		msg += " " + h
	}
	if e.status == http.StatusPaymentRequired || strings.Contains(e.code, "quota") || strings.Contains(e.code, "credit") || strings.Contains(e.code, "limit") {
		msg += " This project is out of credits or over a sending limit. Tell the user; they can see usage with get_account and add credits or wait for the limit to reset in the Forke Mail dashboard. Never try to pay or add credits yourself."
	}
	if e.status == http.StatusTooManyRequests {
		msg += " Slow down and retry shortly."
	}
	if len(e.body) < 400 {
		msg += "\nResponse: " + e.body
	}
	return msg
}

func (c *call) requestApproval(summary string) map[string]any {
	clean := map[string]any{}
	_ = json.Unmarshal([]byte(strings.SplitN(canonical(c.tool.Name, c.args), "\n", 2)[1]), &clean)
	var id string
	err := c.h.Pool.QueryRow(c.ctx, `
		INSERT INTO agent_approvals (workspace_id, requested_by, tool, args, summary, expires_at)
		VALUES ($1, $2, $3, $4, $5, now() + interval '30 minutes') RETURNING id`,
		c.id.WorkspaceID, c.requester(), c.tool.Name, clean, summary).Scan(&id)
	if err != nil {
		return text("Could not create the approval request. Try again.", true)
	}
	link := c.h.BaseURL + "/approve/" + id
	return text(fmt.Sprintf("APPROVAL NEEDED: %s\n\nThis action needs the project owner to approve it in Forke Mail. Give the user this link: %s\nAfter they press Approve, call %s again with exactly the same arguments plus \"approval_id\": %q. The request expires in 30 minutes.", summary, link, c.tool.Name, id), false)
}

func (c *call) redeem(approvalID string) (string, bool) {
	if !httpx.ValidID(approvalID) {
		return "approval_id is not valid.", false
	}
	var status, tool, ws string
	var stored map[string]any
	var expired bool
	err := c.h.Pool.QueryRow(c.ctx, `
		SELECT status, tool, workspace_id, args, expires_at < now() FROM agent_approvals WHERE id = $1`, approvalID,
	).Scan(&status, &tool, &ws, &stored, &expired)
	if errors.Is(err, pgx.ErrNoRows) || ws != c.id.WorkspaceID {
		return "No such approval request in this project.", false
	}
	if err != nil {
		return "Could not check the approval. Try again.", false
	}
	clean := map[string]any{}
	_ = json.Unmarshal([]byte(strings.SplitN(canonical(c.tool.Name, c.args), "\n", 2)[1]), &clean)
	a, _ := json.Marshal(clean)
	b, _ := json.Marshal(stored)
	switch {
	case tool != c.tool.Name || !bytes.Equal(a, b):
		return "This approval was for a different action or different arguments. Call the tool without approval_id to request a new approval.", false
	case status == "denied":
		return "The user declined this action. Do not retry unless they ask.", false
	case status == "used":
		return "This approval was already used. Request a new one if the user wants to do it again.", false
	case expired:
		return "This approval request expired. Call the tool without approval_id to request a new one.", false
	case status == "pending":
		return "Still waiting for the user to approve at " + c.h.BaseURL + "/approve/" + approvalID + ". Ask them to open the link, then call again.", false
	}
	tag, err := c.h.Pool.Exec(c.ctx, `UPDATE agent_approvals SET status = 'used' WHERE id = $1 AND status = 'approved'`, approvalID)
	if err != nil || tag.RowsAffected() == 0 {
		return "This approval was already used.", false
	}
	return "", true
}
