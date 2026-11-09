package tracking

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
	id, target, recipient, err := tr.verifyClick(token)
	if err != nil || recipient || id != "11111111-1111-1111-1111-111111111111" || target != "https://forke.space/reset?t=1&u=2" {
		t.Fatalf("click token %q %q %v", id, target, err)
	}
}

func TestTrackingTokensRejectTamperingAndCrossUse(t *testing.T) {
	tr := NewTracker([]byte(strings.Repeat("k", 32)), "https://mail.test")
	other := NewTracker([]byte(strings.Repeat("z", 32)), "https://mail.test")
	open := tr.sign(openPrefix + "id-1")
	click := tr.sign(clickPrefix + "id-1\nhttps://ok.io")
	forged := tr.sign(clickPrefix + "id-1\njavascript:alert(1)")
	if _, _, err := tr.verifyOpen(open); err != nil {
		t.Fatal(err)
	}
	if _, _, err := other.verifyOpen(open); err == nil {
		t.Error("token verified with another secret")
	}
	if _, _, _, err := tr.verifyClick(open); err == nil {
		t.Error("open token accepted as click")
	}
	if _, _, err := tr.verifyOpen(click); err == nil {
		t.Error("click token accepted as open")
	}
	if _, _, _, err := tr.verifyClick(forged); err == nil {
		t.Error("non http target accepted")
	}
	if _, _, err := tr.verifyOpen(open[:len(open)-2] + "AA"); err == nil {
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

func TestCountableOpens(t *testing.T) {
	for ua, want := range map[string]bool{
		"": false,
		"Mozilla/5.0 (Windows NT 5.1; rv:11.0) Gecko Firefox/11.0 (via ggpht.com GoogleImageProxy)": true,
		"YahooMailProxy; https://help.yahoo.com/kb/yahoo-mail-proxy-SLN28749.html":                  true,
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15":                      true,
		"Mozilla/5.0 (compatible; Barracuda Sentinel)":                                              false,
		"Go-http-client/1.1": false,
	} {
		if got := countableOpen(ua); got != want {
			t.Errorf("countableOpen(%q) = %v", ua, got)
		}
	}
}

func TestRecipientTokensSkipOwnLinks(t *testing.T) {
	tr := NewTracker([]byte(strings.Repeat("k", 32)), "https://mail.test")
	body := `<a href="https://forke.space/blog/x">Read</a> <a href="https://mail.test/unsubscribe/tok">Unsubscribe</a> <a href="https://mail.test/preferences/tok">Prefs</a></body>`
	out := tr.InstrumentRecipient(body, "22222222-2222-2222-2222-222222222222", true, true)
	if !strings.Contains(out, `href="https://mail.test/unsubscribe/tok"`) || !strings.Contains(out, `href="https://mail.test/preferences/tok"`) {
		t.Fatalf("own links were tracked: %s", out)
	}
	start := strings.Index(out, "/t/c/") + len("/t/c/")
	token := out[start : start+strings.Index(out[start:], `"`)]
	id, target, recipient, err := tr.verifyClick(token)
	if err != nil || !recipient || id != "22222222-2222-2222-2222-222222222222" || target != "https://forke.space/blog/x" {
		t.Fatalf("recipient click %q %q %v %v", id, target, recipient, err)
	}
	pixel := out[strings.Index(out, "/t/o/")+len("/t/o/"):]
	pixel = pixel[:strings.Index(pixel, `"`)]
	if id, recipient, err := tr.verifyOpen(pixel); err != nil || !recipient || id != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("recipient open %q %v %v", id, recipient, err)
	}
}
