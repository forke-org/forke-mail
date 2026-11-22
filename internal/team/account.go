// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package team

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Account struct {
	Manage
	SecureCookies bool
}

var errWrongPassword = errors.New("incorrect_password")

func (a *Account) Get(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var out struct {
		Name     string  `json:"name"`
		Email    string  `json:"email"`
		Username *string `json:"username"`
		Forke    bool    `json:"forke"`
		Projects int     `json:"projects"`
	}
	err := a.Pool.QueryRow(r.Context(), `
		SELECT name, email, username, forke_admin_id IS NOT NULL, (SELECT COUNT(*) FROM workspace_members WHERE user_id = u.id)
		FROM users u WHERE id = $1`, id.UserID).Scan(&out.Name, &out.Email, &out.Username, &out.Forke, &out.Projects)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *Account) Update(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Username = strings.ToLower(strings.TrimSpace(body.Username))
	switch {
	case body.Name == "" || utf8.RuneCountInString(body.Name) > 100:
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name")
		return
	case body.Username != "" && !usernamePattern.MatchString(body.Username):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_username")
		return
	}
	tag, err := a.Pool.Exec(r.Context(), `UPDATE users SET name = $2, username = NULLIF($3, '') WHERE id = $1 AND forke_admin_id IS NULL`, id.UserID, body.Name, body.Username)
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		httpx.Error(w, http.StatusConflict, "username_taken")
	case err != nil:
		httpx.Internal(w, r, err)
	case tag.RowsAffected() == 0:
		httpx.Error(w, http.StatusConflict, "account_managed_by_forke")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *Account) Password(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if len(body.New) < 10 || len(body.New) > 72 {
		httpx.Error(w, http.StatusUnprocessableEntity, "weak_password")
		return
	}
	ok, err := (accounts.Passwords{Pool: a.Pool}).Verify(r.Context(), id.UserID, body.Current)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "incorrect_password")
		return
	}
	if err := accounts.SetPassword(r.Context(), a.Pool, id.UserID, body.New); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	c, _ := r.Cookie(auth.SessionCookie)
	keep := ""
	if c != nil {
		keep = auth.HashKey(c.Value)
	}
	if _, err := a.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, id.UserID, keep); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Account) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	var names []string
	var blocking string
	err := pgx.BeginFunc(r.Context(), a.Pool, func(tx pgx.Tx) error {
		var forke bool
		if err := tx.QueryRow(r.Context(), `SELECT forke_admin_id IS NOT NULL FROM users WHERE id = $1 FOR UPDATE`, id.UserID).Scan(&forke); err != nil {
			return err
		}
		if forke {
			return errPlatform
		}
		ok, err := (accounts.Passwords{Pool: tx}).Verify(r.Context(), id.UserID, body.Password)
		if err != nil {
			return err
		}
		if !ok {
			return errWrongPassword
		}
		rows, err := tx.Query(r.Context(), `
			SELECT w.id, w.name, m.role,
				(SELECT COUNT(*) FROM workspace_members o WHERE o.workspace_id = w.id AND o.user_id <> $1),
				(SELECT COUNT(*) FROM workspace_members o WHERE o.workspace_id = w.id AND o.user_id <> $1 AND o.role = 'super_admin')
			FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id WHERE m.user_id = $1`, id.UserID)
		if err != nil {
			return err
		}
		type membership struct {
			ID, Name, Role string
			Others, Owners int
		}
		list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[membership])
		if err != nil {
			return err
		}
		for _, m := range list {
			if m.Others > 0 && m.Role == auth.SuperAdmin && m.Owners == 0 {
				blocking = m.Name
				return errLastOwner
			}
		}
		for _, m := range list {
			if m.Others > 0 {
				if _, err := tx.Exec(r.Context(), `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`, m.ID, id.UserID); err != nil {
					return err
				}
				continue
			}
			rows, err := tx.Query(r.Context(), `SELECT name FROM domains WHERE workspace_id = $1`, m.ID)
			if err != nil {
				return err
			}
			d, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			names = append(names, d...)
			if _, err := tx.Exec(r.Context(), `DELETE FROM workspace_members WHERE workspace_id = $1`, m.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(r.Context(), `DELETE FROM workspaces WHERE id = $1 AND NOT platform`, m.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM access_requests WHERE lower(email) = (SELECT lower(email) FROM users WHERE id = $1)`, id.UserID); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, id.UserID)
		return err
	})
	switch {
	case errors.Is(err, errWrongPassword):
		httpx.Error(w, http.StatusUnauthorized, "incorrect_password")
	case errors.Is(err, errPlatform):
		httpx.Error(w, http.StatusConflict, "account_managed_by_forke")
	case errors.Is(err, errLastOwner):
		httpx.JSON(w, http.StatusConflict, map[string]string{"error": "last_owner", "project": blocking})
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		a.release(r, names)
		_ = auth.EndSession(r.Context(), a.Pool, w, r, a.SecureCookies)
		w.WriteHeader(http.StatusNoContent)
	}
}
