package accounts

import (
	"context"
	"errors"

	"forkemail/internal/db"
	"forkemail/internal/forke"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("forkemail-timing-equalizer"), bcryptCost)

func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(h), err
}

type Passwords struct {
	Pool   db.Querier
	Admins *forke.Directory
}

func (p Passwords) Verify(ctx context.Context, userID, password string) (bool, error) {
	var hash, adminID *string
	err := p.Pool.QueryRow(ctx, `SELECT password_hash, forke_admin_id::text FROM users WHERE id = $1`, userID).Scan(&hash, &adminID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if adminID != nil {
		if p.Admins == nil {
			return false, nil
		}
		return p.Admins.CheckPassword(ctx, *adminID, password)
	}
	return hash != nil && bcrypt.CompareHashAndPassword([]byte(*hash), []byte(password)) == nil, nil
}
