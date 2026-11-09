package transactional

import (
	"context"
	"forkemail/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Submission = request
type Acceptance struct{ ID, Replayed string }
type Service struct {
	Pool       *pgxpool.Pool
	Dispatcher *Dispatcher
}
type validationError struct {
	code   string
	status int
}

func (e *validationError) Error() string { return e.code }
func isValidation(err error) bool        { _, ok := err.(*validationError); return ok }

func (s Service) Accept(ctx context.Context, who auth.Identity, key string, req Submission) (Acceptance, error) {
	if key != "" && !validIdempotencyKey(key) {
		return Acceptance{}, &validationError{"invalid_idempotency_key", 400}
	}
	req.normalize()
	if code := req.validate(); code != "" {
		return Acceptance{}, &validationError{code, 422}
	}
	var res accepted
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { var err error; res, err = accept(ctx, tx, who, key, req); return err })
	if err != nil {
		return Acceptance{}, err
	}
	if res.replayed == "" && s.Dispatcher != nil {
		s.Dispatcher.Notify(res.id)
	}
	return Acceptance{ID: res.id, Replayed: res.replayed}, nil
}
