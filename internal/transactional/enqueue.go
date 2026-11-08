package transactional

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type Request struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

type Job struct {
	ID, WorkspaceID, IdempotencyKey string
	Message                         Request
}
type EmailEnqueuer interface {
	Enqueue(context.Context, pgx.Tx, Job) error
}
type DevelopmentStub struct{}

func (DevelopmentStub) Enqueue(ctx context.Context, tx pgx.Tx, job Job) error {
	if job.ID == "" || job.WorkspaceID == "" || tx == nil {
		return errors.New("invalid_stub_job")
	}
	return ctx.Err()
}
