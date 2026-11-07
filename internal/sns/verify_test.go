package sns_test

import (
	"context"
	"errors"
	"testing"

	"forkemail/internal/sns"
	"forkemail/internal/sns/snstest"
)

func notification() sns.Message {
	return sns.Message{Type: sns.TypeNotification, MessageID: "m-1", Message: `{"eventType":"Delivery"}`}
}

func TestVerifyAcceptsBothSignatureVersions(t *testing.T) {
	signer := snstest.NewSigner(t)
	v := signer.Verifier()
	for _, version := range []string{"1", "2"} {
		m := notification()
		m.SignatureVersion = version
		if err := v.Verify(context.Background(), signer.Sign(t, m)); err != nil {
			t.Errorf("version %s: %v", version, err)
		}
		m.Subject = "with subject"
		if err := v.Verify(context.Background(), signer.Sign(t, m)); err != nil {
			t.Errorf("version %s with subject: %v", version, err)
		}
	}
	confirm := sns.Message{Type: sns.TypeSubscriptionConfirmation, MessageID: "c-1", Token: "tok", Message: "confirm",
		SubscribeURL: "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription&Token=tok"}
	if err := v.Verify(context.Background(), signer.Sign(t, confirm)); err != nil {
		t.Errorf("subscription confirmation: %v", err)
	}
}

func TestVerifyRejectsForgedMessages(t *testing.T) {
	signer := snstest.NewSigner(t)
	other := snstest.NewSigner(t)
	v := signer.Verifier()
	valid := signer.Sign(t, notification())
	cases := map[string]sns.Message{}

	tampered := valid
	tampered.Message = `{"eventType":"Bounce"}`
	cases["tampered message"] = tampered

	tamperedID := valid
	tamperedID.MessageID = "m-2"
	cases["tampered message id"] = tamperedID

	wrongKey := other.Sign(t, notification())
	cases["signed by another key"] = wrongKey

	for name, certURL := range map[string]string{
		"wrong cert host":      "https://evil.example.com/SimpleNotificationService-test.pem",
		"lookalike cert host":  "https://sns.us-east-1.amazonaws.com.evil.com/cert.pem",
		"other region host":    "https://sns.eu-west-1.amazonaws.com/SimpleNotificationService-test.pem",
		"http cert url":        "http://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem",
		"cert path not pem":    "https://sns.us-east-1.amazonaws.com/cert.txt",
		"userinfo in cert url": "https://user@sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem",
	} {
		m := notification()
		m.SigningCertURL = certURL
		cases[name] = signer.Sign(t, m)
	}

	wrongTopic := notification()
	wrongTopic.TopicArn = "arn:aws:sns:us-east-1:123456789012:someone-else"
	cases["wrong topic arn"] = signer.Sign(t, wrongTopic)

	badVersion := valid
	badVersion.SignatureVersion = "3"
	cases["unknown signature version"] = badVersion

	downgrade := valid
	downgrade.SignatureVersion = "1"
	cases["version swapped after signing"] = downgrade

	unknownType := valid
	unknownType.Type = "Other"
	cases["unknown type"] = unknownType

	for name, m := range cases {
		if err := v.Verify(context.Background(), m); !errors.Is(err, sns.ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
	for _, u := range signer.Requests {
		if u != snstest.CertURL {
			t.Errorf("fetched untrusted url %s", u)
		}
	}
}

func TestConfirmOnlyFollowsSNSURLs(t *testing.T) {
	signer := snstest.NewSigner(t)
	v := signer.Verifier()
	for _, u := range []string{
		"https://evil.example.com/?Action=ConfirmSubscription",
		"https://sns.us-east-1.amazonaws.com.evil.com/",
		"http://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription",
		"https://sns.eu-west-1.amazonaws.com/?Action=ConfirmSubscription",
		"https://169.254.169.254/latest/meta-data/",
	} {
		m := sns.Message{Type: sns.TypeSubscriptionConfirmation, SubscribeURL: u}
		if err := v.Confirm(context.Background(), m); !errors.Is(err, sns.ErrUntrustedURL) {
			t.Errorf("%s: got %v, want ErrUntrustedURL", u, err)
		}
	}
	if len(signer.Requests) != 0 {
		t.Fatalf("made requests to untrusted urls: %v", signer.Requests)
	}
	good := "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription&TopicArn=x&Token=y"
	if err := v.Confirm(context.Background(), sns.Message{Type: sns.TypeSubscriptionConfirmation, SubscribeURL: good}); err != nil {
		t.Fatal(err)
	}
	if len(signer.Requests) != 1 || signer.Requests[0] != good {
		t.Fatalf("requests = %v", signer.Requests)
	}
}
