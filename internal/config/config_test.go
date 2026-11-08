package config

import (
	"strings"
	"testing"
)

func base(t *testing.T) {
	for k, v := range map[string]string{
		"APP_ENV": "development", "DATABASE_URL": "postgres://x", "PUBLIC_BASE_URL": "http://localhost:8080",
		"UNSUBSCRIBE_SECRET": strings.Repeat("u", 32), "SEED_TOKEN": strings.Repeat("s", 32),
		"SENDER_DRIVER": "", "AWS_REGION": "", "SES_CONFIGURATION_SET": "", "SNS_TOPIC_ARN": "",
		"AWS_ACCESS_KEY_ID": "", "AWS_SECRET_ACCESS_KEY": "", "MAX_BOUNCE_RATE": "", "SES_TEST_EMAIL": "",
	} {
		t.Setenv(k, v)
	}
}

func TestLogDriverDefaults(t *testing.T) {
	base(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.SenderDriver != "log" || c.MaxBounceRate != 0.05 || c.MaxComplaintRate != 0.001 || c.ReputationMinSample != 100 {
		t.Fatalf("defaults %+v", c)
	}
}

func TestSESDriverValidation(t *testing.T) {
	ses := func(t *testing.T) {
		base(t)
		t.Setenv("SENDER_DRIVER", "ses")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("SES_CONFIGURATION_SET", "forkemail")
		t.Setenv("SNS_TOPIC_ARN", "arn:aws:sns:us-east-1:123456789012:forkemail-ses-events")
		t.Setenv("PUBLIC_BASE_URL", "https://abc.ngrok-free.app")
	}
	ses(t)
	if _, err := Load(); err != nil {
		t.Fatalf("valid ses config rejected: %v", err)
	}
	bad := map[string][2]string{
		"http base url":      {"PUBLIC_BASE_URL", "http://abc.ngrok-free.app"},
		"trailing slash":     {"PUBLIC_BASE_URL", "https://abc.ngrok-free.app/"},
		"missing region":     {"AWS_REGION", ""},
		"missing topic":      {"SNS_TOPIC_ARN", ""},
		"topic other region": {"SNS_TOPIC_ARN", "arn:aws:sns:eu-west-1:123456789012:t"},
		"missing config set": {"SES_CONFIGURATION_SET", ""},
		"half credentials":   {"AWS_ACCESS_KEY_ID", "AKIAEXAMPLE"},
		"bounce rate > 1":    {"MAX_BOUNCE_RATE", "2"},
		"unknown driver":     {"SENDER_DRIVER", "smtp"},
	}
	for name, kv := range bad {
		t.Run(name, func(t *testing.T) {
			ses(t)
			t.Setenv(kv[0], kv[1])
			if _, err := Load(); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
