package transactional

import (
	"strings"
	"testing"
)

func TestInstrumentRewritesHTTPLinksAndAddsPixel(t *testing.T) {
	tr := NewTracker([]byte(strings.Repeat("k", 32)), "https://mail.test")
	body := `<html><body><a href="https://forke.space/reset?t=1&amp;u=2">Reset</a> <a href='mailto:x@y.io'>Mail</a> <a class="x" href="http://a.io/b">B</a></body></html>`
	out := tr.Instrument(body, "11111111-1111-1111-1111-111111111111", true, true)
	if strings.Contains(out, "forke.space/reset") || strings.Contains(out, `"http://a.io/b"`) {
		t.Fatalf("http links not rewritten: %s", out)
	}
	if !strings.Contains(out, "mailto:x@y.io") {
		t.Fatal("mailto link was rewritten")
	}
	if strings.Count(out, "https://mail.test/t/c/") != 2 || strings.Count(out, "https://mail.test/t/o/") != 1 {
		t.Fatalf("unexpected tracking urls: %s", out)
	}
	if !strings.HasSuffix(out, "</body></html>") {
		t.Fatal("pixel not placed before </body>")
	}
	start := strings.Index(out, "/t/c/") + len("/t/c/")
	token := out[start : start+strings.Index(out[start:], `"`)]
	id, target, err := tr.verifyClick(token)
	if err != nil || id != "11111111-1111-1111-1111-111111111111" || target != "https://forke.space/reset?t=1&u=2" {
		t.Fatalf("click token %q %q %v", id, target, err)
	}
}

func TestTrackingTokensRejectTamperingAndCrossUse(t *testing.T) {
	tr := NewTracker([]byte(strings.Repeat("k", 32)), "https://mail.test")
	other := NewTracker([]byte(strings.Repeat("z", 32)), "https://mail.test")
	open := tr.sign(openPrefix + "id-1")
	click := tr.sign(clickPrefix + "id-1\nhttps://ok.io")
	forged := tr.sign(clickPrefix + "id-1\njavascript:alert(1)")
	if _, err := tr.verifyOpen(open); err != nil {
		t.Fatal(err)
	}
	if _, err := other.verifyOpen(open); err == nil {
		t.Error("token verified with another secret")
	}
	if _, _, err := tr.verifyClick(open); err == nil {
		t.Error("open token accepted as click")
	}
	if _, err := tr.verifyOpen(click); err == nil {
		t.Error("click token accepted as open")
	}
	if _, _, err := tr.verifyClick(forged); err == nil {
		t.Error("non http target accepted")
	}
	if _, err := tr.verifyOpen(open[:len(open)-2] + "AA"); err == nil {
		t.Error("tampered signature accepted")
	}
}

func TestAutomatedAgents(t *testing.T) {
	for ua, want := range map[string]bool{
		"": true,
		"Mozilla/5.0 (Windows NT 5.1; rv:11.0) Gecko Firefox/11.0 (via ggpht.com GoogleImageProxy)": true,
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15":                      false,
		"Go-http-client/1.1": true,
	} {
		if got := automated(ua); got != want {
			t.Errorf("automated(%q) = %v", ua, got)
		}
	}
}
