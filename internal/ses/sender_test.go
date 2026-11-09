package ses

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"forkemail/internal/sender"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func responded(status int, err error) error {
	return &smithy.OperationError{ServiceID: "SESv2", OperationName: "SendEmail", Err: &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}}, Err: err,
	}}
}

func apiError(status int, code, message string) error {
	return responded(status, &smithy.GenericAPIError{Code: code, Message: message})
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		action sender.Action
		reason string
	}{
		{"throttling", apiError(429, "TooManyRequestsException", "slow down"), sender.Retry, "throttled"},
		{"limit exceeded", apiError(400, "LimitExceededException", "max rate"), sender.Retry, "throttled"},
		{"legacy throttling", apiError(400, "Throttling", "rate"), sender.Retry, "throttled"},
		{"internal error", apiError(500, "InternalServiceErrorException", "oops"), sender.Retry, "ses_unavailable"},
		{"bare 503", responded(503, errors.New("service unavailable")), sender.Retry, "ses_unavailable"},
		{"typed sending paused", responded(400, &types.SendingPausedException{Message: aws.String("paused")}), sender.PauseSending, "ses_sending_paused"},
		{"account sending paused", apiError(400, "AccountSendingPaused", ""), sender.PauseSending, "ses_sending_paused"},
		{"config set sending paused", apiError(400, "ConfigurationSetSendingPaused", ""), sender.PauseSending, "ses_sending_paused"},
		{"account suspended", apiError(400, "AccountSuspendedException", ""), sender.PauseSending, "ses_sending_paused"},
		{"mail from not verified", apiError(400, "MailFromDomainNotVerifiedException", ""), sender.PauseIdentity, "identity_not_verified"},
		{"identity not found", apiError(404, "NotFoundException", ""), sender.PauseIdentity, "identity_not_verified"},
		{"sandbox address", apiError(400, "MessageRejected", "Email address is not verified."), sender.Fail, "message_rejected: Email address is not verified."},
		{"bad request", apiError(400, "BadRequestException", "bad"), sender.Fail, "ses_error: BadRequestException"},
		{"network timeout", &smithy.OperationError{Err: &net.OpError{Op: "read", Err: timeoutError{}}}, sender.UnknownOutcome, "unknown_outcome"},
		{"context deadline", context.DeadlineExceeded, sender.UnknownOutcome, "unknown_outcome"},
		{"connection reset", errors.New("connection reset by peer"), sender.UnknownOutcome, "unknown_outcome"},
	}
	for _, c := range cases {
		got := Classify(c.err)
		if got.Action != c.action || got.Reason != c.reason {
			t.Errorf("%s: got %d %q, want %d %q", c.name, got.Action, got.Reason, c.action, c.reason)
		}
		if !errors.Is(got, c.err) {
			t.Errorf("%s: original error not wrapped", c.name)
		}
	}
}

type fakeClient struct {
	in      *sesv2.SendEmailInput
	options sesv2.Options
	out     *sesv2.SendEmailOutput
	err     error
}

func (f *fakeClient) SendEmail(ctx context.Context, in *sesv2.SendEmailInput, optFns ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	f.in = in
	for _, fn := range optFns {
		fn(&f.options)
	}
	return f.out, f.err
}

func TestSendBuildsRequest(t *testing.T) {
	fake := &fakeClient{out: &sesv2.SendEmailOutput{MessageId: aws.String("ses-123")}}
	s := Sender{Client: fake, ConfigurationSet: "forkemail"}
	id, err := s.Send(context.Background(), sender.Message{
		WorkspaceID: "ws", EmailID: "em", RecipientID: "rc", To: "to@x.io", From: `"Forke" <hi@forke.space>`,
		Subject: "Hello", HTML: "<p>hi</p>", Text: "hi",
		Headers: map[string]string{"List-Unsubscribe": "<https://m.x/u/t>", "List-Unsubscribe-Post": "List-Unsubscribe=One-Click", "To": "to@x.io"},
	})
	if err != nil || id != "ses-123" {
		t.Fatalf("id %q err %v", id, err)
	}
	in := fake.in
	if aws.ToString(in.FromEmailAddress) != `"Forke" <hi@forke.space>` || aws.ToString(in.ConfigurationSetName) != "forkemail" ||
		len(in.Destination.ToAddresses) != 1 || in.Destination.ToAddresses[0] != "to@x.io" {
		t.Fatalf("envelope %+v", in)
	}
	msg := in.Content.Simple
	if aws.ToString(msg.Subject.Data) != "Hello" || aws.ToString(msg.Body.Html.Data) != "<p>hi</p>" || aws.ToString(msg.Body.Text.Data) != "hi" {
		t.Fatalf("content %+v", msg)
	}
	headers := map[string]string{}
	for _, h := range msg.Headers {
		headers[aws.ToString(h.Name)] = aws.ToString(h.Value)
	}
	if len(headers) != 2 || headers["List-Unsubscribe"] != "<https://m.x/u/t>" || headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Fatalf("headers %v", headers)
	}
	tags := map[string]string{}
	for _, tag := range in.EmailTags {
		tags[aws.ToString(tag.Name)] = aws.ToString(tag.Value)
	}
	if tags["workspace_id"] != "ws" || tags["email_id"] != "em" || tags["recipient_id"] != "rc" {
		t.Fatalf("tags %v", tags)
	}
	if _, ok := fake.options.Retryer.(aws.NopRetryer); !ok {
		t.Fatalf("sdk retries not disabled: %T", fake.options.Retryer)
	}

	fake.err = apiError(400, "MessageRejected", "nope")
	if _, err := s.Send(context.Background(), sender.Message{}); err == nil {
		t.Fatal("expected error")
	} else if se := (*sender.SendError)(nil); !errors.As(err, &se) || se.Action != sender.Fail {
		t.Fatalf("error not classified: %v", err)
	}
}

func TestInputDerivesTextFromHTML(t *testing.T) {
	in := Sender{ConfigurationSet: "cs"}.input(sender.Message{From: "a@forke.space", To: "b@x.io", Subject: "s", HTML: `<p>Hello <a href="https://forke.space">Forke</a></p>`})
	if in.Content.Simple.Body.Text == nil || *in.Content.Simple.Body.Text.Data != "Hello Forke (https://forke.space)\n" {
		t.Fatalf("text part %v", in.Content.Simple.Body.Text)
	}
}
