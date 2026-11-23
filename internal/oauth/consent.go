// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package oauth

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"forkemail/frontend"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"forkemail/internal/team"

	"github.com/jackc/pgx/v5"
)

//go:embed consent.html
var consentHTML string

var consentPage = template.Must(template.New("consent").Funcs(template.FuncMap{"asset": frontend.Asset}).Parse(consentHTML))

type project struct{ ID, Name string }

type consentView struct {
	Client   string
	Email    string
	Projects []project
	Current  string
	Params   map[string]string
	Error    string
}

func jsonDecode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

type authRequest struct {
	ClientID, RedirectURI, Challenge, State string
}

func (s *Server) parseAuthRequest(ctx context.Context, v url.Values) (authRequest, client, string) {
	req := authRequest{ClientID: v.Get("client_id"), RedirectURI: v.Get("redirect_uri"), Challenge: v.Get("code_challenge"), State: v.Get("state")}
	c, err := s.client(ctx, req.ClientID)
	if err != nil {
		return req, c, "This app is not registered with Forke Mail. Remove it from your AI app and add it again."
	}
	if !redirectMatches(c.Redirects, req.RedirectURI) {
		return req, c, "This app asked to return to an address it did not register."
	}
	if v.Get("response_type") != "code" || v.Get("code_challenge_method") != "S256" || len(req.Challenge) < 43 || len(req.Challenge) > 128 {
		return req, c, "This app sent an unsupported sign-in request (Forke Mail needs PKCE with S256)."
	}
	return req, c, ""
}

func (s *Server) session(r *http.Request) (auth.Identity, bool) {
	ck, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		return auth.Identity{}, false
	}
	id, err := s.Auth.IdentifySession(r.Context(), ck.Value)
	return id, err == nil
}

func (s *Server) Authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req, c, problem := s.parseAuthRequest(r.Context(), q)
	if problem != "" {
		s.render(w, http.StatusBadRequest, consentView{Error: problem})
		return
	}
	id, ok := s.session(r)
	if !ok {
		http.Redirect(w, r, "/login/?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	var email string
	if err := s.Pool.QueryRow(r.Context(), `SELECT email FROM users WHERE id = $1`, id.UserID).Scan(&email); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := s.Pool.Query(r.Context(), `
		SELECT w.id, w.name FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND w.status = 'active' ORDER BY lower(w.name)`, id.UserID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	projects, err := pgx.CollectRows(rows, pgx.RowToStructByPos[project])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	params := map[string]string{"client_id": req.ClientID, "redirect_uri": req.RedirectURI, "code_challenge": req.Challenge,
		"code_challenge_method": "S256", "response_type": "code", "state": req.State}
	if target, err := url.Parse(req.RedirectURI); err == nil {
		source := target.Scheme + ":"
		if target.Host != "" {
			source = target.Scheme + "://" + target.Host
		}
		httpx.AllowFormTarget(w, source)
	}
	s.render(w, http.StatusOK, consentView{Client: c.Name, Email: email, Projects: projects, Current: id.WorkspaceID, Params: params})
}

func (s *Server) Decide(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil || !httpx.SameOrigin(r) {
		s.render(w, http.StatusForbidden, consentView{Error: "This approval did not come from the Forke Mail page."})
		return
	}
	req, _, problem := s.parseAuthRequest(r.Context(), r.PostForm)
	if problem != "" {
		s.render(w, http.StatusBadRequest, consentView{Error: problem})
		return
	}
	id, ok := s.session(r)
	if !ok {
		s.render(w, http.StatusUnauthorized, consentView{Error: "Your session ended. Start the connection again from your AI app."})
		return
	}
	target, _ := url.Parse(req.RedirectURI)
	out := target.Query()
	if req.State != "" {
		out.Set("state", req.State)
	}
	if r.PostForm.Get("decision") != "allow" {
		out.Set("error", "access_denied")
		target.RawQuery = out.Encode()
		http.Redirect(w, r, target.String(), http.StatusSeeOther)
		return
	}
	workspace := r.PostForm.Get("workspace_id")
	if workspace == "new" && s.Projects != nil {
		created, err := s.Projects.CreateFor(r.Context(), id.UserID, r.PostForm.Get("project_name"))
		switch {
		case errors.Is(err, team.ErrTooManyProjects):
			s.render(w, http.StatusConflict, consentView{Error: "You already have the most projects your plan allows. Pick an existing project instead."})
			return
		case errors.Is(err, team.ErrInvalidProjectName):
			s.render(w, http.StatusUnprocessableEntity, consentView{Error: "Give the new project a name of up to 100 characters."})
			return
		case err != nil:
			httpx.Internal(w, r, err)
			return
		}
		workspace = created
	}
	var member bool
	if err := s.Pool.QueryRow(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
			WHERE m.user_id = $1 AND m.workspace_id::text = $2 AND w.status = 'active')`, id.UserID, workspace).Scan(&member); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !member {
		s.render(w, http.StatusForbidden, consentView{Error: "You are not a member of that project."})
		return
	}
	code := auth.RandomHex(32)
	if _, err := s.Pool.Exec(r.Context(), `
		INSERT INTO oauth_codes (code_hash, client_id, user_id, workspace_id, redirect_uri, challenge, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, auth.HashKey(code), req.ClientID, id.UserID, workspace, req.RedirectURI, req.Challenge, time.Now().Add(codeLifetime)); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	out.Set("code", code)
	target.RawQuery = out.Encode()
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func (s *Server) render(w http.ResponseWriter, status int, v consentView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = consentPage.Execute(w, v)
}
