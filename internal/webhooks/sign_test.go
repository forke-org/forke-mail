package webhooks

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func testVault(t *testing.T) *Vault {
	t.Helper()
	v, err := NewVault(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestSignatureAndReplayWindow(t *testing.T) {
	body := []byte(`{"type":"email.delivered"}`)
	now := time.Unix(1000, 0)
	sig := Signature("secret", "delivery", "1000", body)
	if !Verify("secret", "delivery", "1000", sig, body, now) {
		t.Fatal("valid signature rejected")
	}
	if Verify("secret", "delivery", "1000", sig, []byte(`{}`), now) || Verify("secret", "other", "1000", sig, body, now) || Verify("secret", "delivery", "1000", sig, body, now.Add(301*time.Second)) {
		t.Fatal("tampering or stale replay accepted")
	}
	v := testVault(t)
	encrypted := v.Seal("secret", "workspace")
	if encrypted == "secret" {
		t.Fatal("not encrypted")
	}
	plain, err := v.Open(encrypted, "workspace")
	if err != nil || plain != "secret" {
		t.Fatal("decrypt failed")
	}
	if _, err := v.Open(encrypted, "another-workspace"); err == nil {
		t.Fatal("workspace substitution accepted")
	}
}
