// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	codeLifetime    = 10 * time.Minute
	accessLifetime  = time.Hour
	refreshLifetime = 60 * 24 * time.Hour
	refreshPrefix   = "fm_refresh_"
	maxRedirectURIs = 10
)

var customScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,40}$`)

type Server struct {
	Pool    *pgxpool.Pool
	Auth    *auth.Authenticator
	BaseURL string
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Protocol-Version")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func (s *Server) Preflight(w http.ResponseWriter, _ *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	cors(w)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"resource":                 s.BaseURL + "/mcp",
		"authorization_servers":    []string{s.BaseURL},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"mail"},
		"resource_name":            "Forke Mail",
		"resource_documentation":   s.BaseURL + "/llms.txt",
	})
}

func (s *Server) ServerMetadata(w http.ResponseWriter, _ *http.Request) {
	cors(w)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"issuer":                                s.BaseURL,
		"authorization_endpoint":                s.BaseURL + "/oauth/authorize",
		"token_endpoint":                        s.BaseURL + "/oauth/token",
		"registration_endpoint":                 s.BaseURL + "/oauth/register",
		"revocation_endpoint":                   s.BaseURL + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mail"},
	})
}

func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || len(raw) > 500 {
		return false
	}
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		return loopback(u.Hostname())
	case "javascript", "data", "file", "vbscript", "blob", "about":
		return false
	}
	return customScheme.MatchString(u.Scheme)
}

func loopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func redirectMatches(registered []string, given string) bool {
	g, err := url.Parse(given)
	if err != nil {
		return false
	}
	for _, r := range registered {
		if r == given {
			return true
		}
		u, err := url.Parse(r)
		if err == nil && u.Scheme == "http" && g.Scheme == "http" && loopback(u.Hostname()) && u.Hostname() == g.Hostname() && u.Path == g.Path {
			return true
		}
	}
	return false
}

func (s *Server) Register(w http.ResponseWriter, r *http.Request) {
	cors(w)
	var body struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := jsonDecode(r, &body); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	body.ClientName = strings.TrimSpace(body.ClientName)
	if body.ClientName == "" {
		body.ClientName = "AI app"
	}
	if utf8.RuneCountInString(body.ClientName) > 100 {
		body.ClientName = string([]rune(body.ClientName)[:100])
	}
	if len(body.RedirectURIs) == 0 || len(body.RedirectURIs) > maxRedirectURIs {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri")
		return
	}
	for _, u := range body.RedirectURIs {
		if !validRedirect(u) {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri")
			return
		}
	}
	id := "fmc_" + auth.RandomHex(16)
	if _, err := s.Pool.Exec(r.Context(), `INSERT INTO oauth_clients (id, name, redirect_uris) VALUES ($1, $2, $3)`, id, body.ClientName, body.RedirectURIs); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"client_id": id, "client_name": body.ClientName, "redirect_uris": body.RedirectURIs,
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
		"client_id_issued_at": time.Now().Unix(),
	})
}

type client struct {
	ID, Name  string
	Redirects []string
}

func (s *Server) client(ctx context.Context, id string) (client, error) {
	c := client{ID: id}
	err := s.Pool.QueryRow(ctx, `SELECT name, redirect_uris FROM oauth_clients WHERE id = $1`, id).Scan(&c.Name, &c.Redirects)
	return c, err
}

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Server) Token(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.exchangeCode(w, r)
	case "refresh_token":
		s.refresh(w, r)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type")
	}
}

func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request) {
	f := r.PostForm
	var userID, workspaceID, clientID, redirect, challenge string
	err := pgx.BeginFunc(r.Context(), s.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(r.Context(), `
			DELETE FROM oauth_codes WHERE code_hash = $1 AND expires_at > now()
			RETURNING user_id, workspace_id, client_id, redirect_uri, challenge`, auth.HashKey(f.Get("code")),
		).Scan(&userID, &workspaceID, &clientID, &redirect, &challenge)
		if err != nil {
			return err
		}
		if clientID != f.Get("client_id") || redirect != f.Get("redirect_uri") || challengeOf(f.Get("code_verifier")) != challenge {
			return errInvalidGrant
		}
		return s.issue(w, r, tx, "", clientID, userID, workspaceID)
	})
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, errInvalidGrant) {
		oauthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
	}
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	token := r.PostForm.Get("refresh_token")
	err := pgx.BeginFunc(r.Context(), s.Pool, func(tx pgx.Tx) error {
		var grantID, clientID, userID, workspaceID string
		err := tx.QueryRow(r.Context(), `
			SELECT id, client_id, user_id, workspace_id FROM oauth_grants
			WHERE refresh_hash = $1 AND refresh_expires_at > now() FOR UPDATE`, auth.HashKey(token),
		).Scan(&grantID, &clientID, &userID, &workspaceID)
		if err != nil {
			return err
		}
		if c := r.PostForm.Get("client_id"); c != "" && c != clientID {
			return errInvalidGrant
		}
		return s.issue(w, r, tx, grantID, clientID, userID, workspaceID)
	})
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, errInvalidGrant) {
		oauthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
	}
}

var errInvalidGrant = errors.New("invalid_grant")

func (s *Server) issue(w http.ResponseWriter, r *http.Request, tx pgx.Tx, grantID, clientID, userID, workspaceID string) error {
	access := auth.GrantPrefix + auth.RandomHex(32)
	refresh := refreshPrefix + auth.RandomHex(32)
	now := time.Now()
	var err error
	if grantID == "" {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO oauth_grants (client_id, user_id, workspace_id, access_hash, refresh_hash, access_expires_at, refresh_expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, clientID, userID, workspaceID, auth.HashKey(access), auth.HashKey(refresh), now.Add(accessLifetime), now.Add(refreshLifetime))
	} else {
		_, err = tx.Exec(r.Context(), `
			UPDATE oauth_grants SET access_hash = $2, refresh_hash = $3, access_expires_at = $4, refresh_expires_at = $5 WHERE id = $1`,
			grantID, auth.HashKey(access), auth.HashKey(refresh), now.Add(accessLifetime), now.Add(refreshLifetime))
	}
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "Bearer", "expires_in": int(accessLifetime.Seconds()),
		"refresh_token": refresh, "scope": "mail",
	})
	return nil
}

func (s *Server) Revoke(w http.ResponseWriter, r *http.Request) {
	cors(w)
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	hash := auth.HashKey(r.PostForm.Get("token"))
	if _, err := s.Pool.Exec(r.Context(), `DELETE FROM oauth_grants WHERE access_hash = $1 OR refresh_hash = $1`, hash); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func oauthError(w http.ResponseWriter, status int, code string) {
	httpx.JSON(w, status, map[string]string{"error": code})
}

func (s *Server) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := s.Clean(ctx); err != nil {
			slog.Warn("oauth cleanup", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) Clean(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `
		WITH codes AS (DELETE FROM oauth_codes WHERE expires_at < now()),
			grants AS (DELETE FROM oauth_grants WHERE refresh_expires_at < now()),
			approvals AS (DELETE FROM agent_approvals WHERE created_at < now() - interval '90 days')
		DELETE FROM oauth_clients c WHERE c.created_at < now() - interval '30 days'
			AND NOT EXISTS (SELECT 1 FROM oauth_grants g WHERE g.client_id = c.id)
			AND NOT EXISTS (SELECT 1 FROM oauth_codes o WHERE o.client_id = c.id)`)
	return err
}
