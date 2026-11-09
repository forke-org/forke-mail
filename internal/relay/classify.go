package relay

import (
	"errors"
	"net/textproto"
	"strconv"

	"forkemail/internal/sender"
)

func classify(err error) *sender.SendError {
	var reply *textproto.Error
	if !errors.As(err, &reply) {
		return &sender.SendError{Action: sender.Retry, Reason: "smtp_unavailable", Err: err}
	}
	code := strconv.Itoa(reply.Code)
	switch {
	case reply.Code == 530 || reply.Code == 534 || reply.Code == 535:
		return &sender.SendError{Action: sender.PauseSending, Reason: "smtp_auth_failed", Err: err}
	case reply.Code >= 400 && reply.Code < 500:
		return &sender.SendError{Action: sender.Retry, Reason: "smtp_" + code, Err: err}
	}
	return &sender.SendError{Action: sender.Fail, Reason: "smtp_rejected: " + code + " " + reply.Msg, Err: err}
}

func unknownOutcome(err error) *sender.SendError {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return classify(err)
	}
	return &sender.SendError{Action: sender.UnknownOutcome, Reason: "unknown_outcome", Err: err}
}
