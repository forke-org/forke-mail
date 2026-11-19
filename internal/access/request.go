package access

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"

	"forkemail/internal/contacts"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Public struct {
	Pool          *pgxpool.Pool
	SecureCookies bool
	Workspace     WorkspaceDefaults
}

type WorkspaceDefaults struct {
	SendLimit  int
	DailyLimit int
	RatePerSec int
}

func newToken() (string, string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (p *Public) Request(w http.ResponseWriter, r *http.Request) {
	if !httpx.SameOrigin(r) {
		httpx.Error(w, http.StatusForbidden, "cross_origin_request")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	email := contacts.NormalizeEmail(body.Email)
	if !contacts.ValidEmail(email) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	if _, err := p.Pool.Exec(r.Context(), `
		INSERT INTO access_requests (email)
		SELECT $1 WHERE NOT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)
		ON CONFLICT (lower(email)) DO NOTHING`, email); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}
