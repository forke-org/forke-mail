package unsub

import (
	"strings"
	"testing"
)

var target = Target{
	WorkspaceID: "11111111-1111-1111-1111-111111111111",
	ContactID:   "22222222-2222-2222-2222-222222222222",
	EmailID:     "33333333-3333-3333-3333-333333333333",
}

func TestTokenRoundTrip(t *testing.T) {
	s := Signer{Secret: []byte(strings.Repeat("s", 32))}
	got, err := s.Verify(s.Sign(target))
	if err != nil || got != target {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestTokenTampering(t *testing.T) {
	s := Signer{Secret: []byte(strings.Repeat("s", 32))}
	token := s.Sign(target)
	payload, sig, _ := strings.Cut(token, ".")
	other := s.Sign(Target{WorkspaceID: target.WorkspaceID, ContactID: target.EmailID, EmailID: target.EmailID})
	otherPayload, _, _ := strings.Cut(other, ".")
	flip := func(s string, i int) string {
		b := []byte(s)
		if b[i] == 'A' {
			b[i] = 'B'
		} else {
			b[i] = 'A'
		}
		return string(b)
	}
	bad := []string{
		"",
		payload,
		payload + ".",
		otherPayload + "." + sig,
		payload + "." + flip(sig, 0),
		payload + "." + flip(sig, len(sig)-1),
		flip(payload, len(payload)-1) + "." + sig,
		token + "x",
		Signer{Secret: []byte(strings.Repeat("t", 32))}.Sign(target),
	}
	for _, b := range bad {
		if _, err := s.Verify(b); err == nil {
			t.Errorf("accepted tampered token %q", b)
		}
	}
}
