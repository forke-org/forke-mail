package transactional

import (
	"context"
	"errors"
	"forkemail/internal/auth"
	"forkemail/internal/templates"
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
	if req.TemplateID == "" {
		req.TemplateID = req.Template
	}
	if req.TemplateID != "" {
		if len(req.Data) > 100 {
			return Acceptance{}, &validationError{"too_many_variables", 422}
		}
		subject, html, text, err := templates.Render(ctx, s.Pool, who.WorkspaceID, req.TemplateID, req.Data)
		if errors.Is(err, templates.ErrNotFound) {
			return Acceptance{}, &validationError{"template_not_found", 404}
		}
		if err != nil {
			return Acceptance{}, err
		}
		if req.Subject == "" {
			req.Subject = subject
		}
		if req.HTML == "" && req.Text == "" {
			req.HTML, req.Text = html, text
		}
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
