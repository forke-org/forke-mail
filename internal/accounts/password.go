package accounts

import (
	"context"
	"errors"

	"forkemail/internal/db"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	maxPasswordBytes  = 72
	bcryptCost        = 12
)

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("forkemail-timing-equalizer"), bcryptCost)

func hashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(h), err
}

func validPassword(password string) bool {
	return len(password) >= minPasswordLength && len(password) <= maxPasswordBytes
}

func VerifyPassword(ctx context.Context, q db.Querier, userID, password string) (bool, error) {
	var hash string
	err := q.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}
