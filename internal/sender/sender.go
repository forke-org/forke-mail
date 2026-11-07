package sender

import "context"

type Message struct {
	WorkspaceID string            `json:"workspace_id"`
	EmailID     string            `json:"email_id"`
	RecipientID string            `json:"recipient_id"`
	To          string            `json:"to"`
	From        string            `json:"from"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html"`
	Text        string            `json:"text"`
	Headers     map[string]string `json:"headers"`
}

type Sender interface {
	Send(ctx context.Context, m Message) (messageID string, err error)
}

type Simulator interface {
	Simulate(ctx context.Context, m Message, messageID string)
}

type Action int

const (
	Retry Action = iota + 1
	Fail
	PauseSending
	PauseIdentity
	UnknownOutcome
)

type SendError struct {
	Action Action
	Reason string
	Err    error
}

func (e *SendError) Error() string {
	return e.Reason + ": " + e.Err.Error()
}

func (e *SendError) Unwrap() error {
	return e.Err
}
