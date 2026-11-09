package ses

import (
	"context"
	"errors"
	"slices"
	"strings"

	"forkemail/internal/sender"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type sendAPI interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

type Sender struct {
	Client           sendAPI
	ConfigurationSet string
}

func (s Sender) Send(ctx context.Context, m sender.Message) (string, error) {
	out, err := s.Client.SendEmail(ctx, s.input(m), func(o *sesv2.Options) {
		o.Retryer = aws.NopRetryer{}
	})
	if err != nil {
		return "", Classify(err)
	}
	if out.MessageId == nil || *out.MessageId == "" {
		return "", &sender.SendError{Action: sender.UnknownOutcome, Reason: "unknown_outcome", Err: errors.New("ses returned no message id")}
	}
	return *out.MessageId, nil
}

func (s Sender) input(m sender.Message) *sesv2.SendEmailInput {
	utf8 := aws.String("UTF-8")
	body := &types.Body{}
	if m.HTML != "" {
		body.Html = &types.Content{Data: aws.String(m.HTML), Charset: utf8}
	}
	text := m.Text
	if strings.TrimSpace(text) == "" && m.HTML != "" {
		text = sender.PlainText(m.HTML)
	}
	if text != "" {
		body.Text = &types.Content{Data: aws.String(text), Charset: utf8}
	}
	var headers []types.MessageHeader
	for _, name := range []string{"List-Unsubscribe", "List-Unsubscribe-Post"} {
		if v := m.Headers[name]; v != "" {
			headers = append(headers, types.MessageHeader{Name: aws.String(name), Value: aws.String(v)})
		}
	}
	names := make([]string, 0, 3)
	tags := m.Tags()
	for name := range tags {
		names = append(names, name)
	}
	slices.Sort(names)
	emailTags := make([]types.MessageTag, len(names))
	for i, name := range names {
		emailTags[i] = types.MessageTag{Name: aws.String(name), Value: aws.String(tags[name])}
	}
	var replyTo []string
	if m.ReplyTo != "" {
		replyTo = []string{m.ReplyTo}
	}
	return &sesv2.SendEmailInput{
		FromEmailAddress:     aws.String(m.From),
		Destination:          &types.Destination{ToAddresses: []string{m.To}},
		ReplyToAddresses:     replyTo,
		ConfigurationSetName: aws.String(s.ConfigurationSet),
		Content: &types.EmailContent{Simple: &types.Message{
			Subject: &types.Content{Data: aws.String(m.Subject), Charset: utf8},
			Body:    body,
			Headers: headers,
		}},
		EmailTags: emailTags,
	}
}

func Classify(err error) *sender.SendError {
	fail := func(action sender.Action, reason string) *sender.SendError {
		return &sender.SendError{Action: action, Reason: reason, Err: err}
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		switch {
		case code == "TooManyRequestsException", code == "LimitExceededException", code == "Throttling", code == "ThrottlingException":
			return fail(sender.Retry, "throttled")
		case strings.HasSuffix(code, "SendingPausedException"), code == "AccountSendingPaused",
			code == "ConfigurationSetSendingPaused", code == "AccountSuspendedException":
			return fail(sender.PauseSending, "ses_sending_paused")
		case code == "MailFromDomainNotVerifiedException", code == "NotFoundException":
			return fail(sender.PauseIdentity, "identity_not_verified")
		case code == "MessageRejected":
			return fail(sender.Fail, "message_rejected: "+apiErr.ErrorMessage())
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		if respErr.HTTPStatusCode() >= 500 {
			return fail(sender.Retry, "ses_unavailable")
		}
		code := "request_failed"
		if apiErr != nil {
			code = apiErr.ErrorCode()
		}
		return fail(sender.Fail, "ses_error: "+code)
	}
	return fail(sender.UnknownOutcome, "unknown_outcome")
}
