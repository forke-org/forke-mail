package forke

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
)

var (
	ErrInvalidCredentials = errors.New("invalid_credentials")
	ErrInactive           = errors.New("admin_inactive")
)

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("forke-mail-timing-equalizer"), 10)

type Admin struct {
	ID    string
	Email string
	Name  string
	Role  string
}

type Directory struct {
	Pool *pgxpool.Pool
}

type record struct {
	Admin
	hash     string
	disabled bool
}

func (r record) active() bool {
	return !r.disabled && r.hash != "" && (r.Role == RoleSuperAdmin || r.Role == RoleAdmin)
}

func (d Directory) Authenticate(ctx context.Context, login, password string) (Admin, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	r, err := d.find(ctx, `lower(email) = $1 OR lower(username) = $1 ORDER BY (lower(email) = $1) DESC LIMIT 1`, login)
	return r.Admin, d.verify(r, err, password)
}

func (d Directory) CheckPassword(ctx context.Context, adminID, password string) (bool, error) {
	r, err := d.find(ctx, `id = $1`, adminID)
	err = d.verify(r, err, password)
	if errors.Is(err, ErrInvalidCredentials) {
		return false, nil
	}
	return err == nil, err
}

func (d Directory) Role(ctx context.Context, adminID string) (string, error) {
	r, err := d.find(ctx, `id = $1`, adminID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !r.active()) {
		return "", ErrInactive
	}
	return r.Role, err
}

func (d Directory) VisibleAdmins(ctx context.Context) (int, error) {
	var n int
	err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&n)
	return n, err
}

func (d Directory) find(ctx context.Context, where string, arg any) (record, error) {
	var r record
	err := d.Pool.QueryRow(ctx, `
		SELECT id::text, email, name, COALESCE(password_hash, ''), role, is_disabled
		FROM admins WHERE `+where, arg).Scan(&r.ID, &r.Email, &r.Name, &r.hash, &r.Role, &r.disabled)
	return r, err
}

func (d Directory) verify(r record, err error, password string) error {
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	hash := dummyHash
	if err == nil && r.hash != "" {
		hash = []byte(r.hash)
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
	if err != nil || !match || !r.active() {
		return ErrInvalidCredentials
	}
	return nil
}
