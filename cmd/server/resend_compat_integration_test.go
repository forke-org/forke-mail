package main

import (
	"encoding/json"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
)

func (f apiFixture) key(t *testing.T, scope string) string {
	t.Helper()
	w := f.request("POST", "/api/keys", `{"name":"`+scope+`","scope":"`+scope+`","password":"test-password"}`, "")
	var k struct{ Key string }
	if err := json.Unmarshal(w.Body.Bytes(), &k); err != nil || w.Code != 201 {
		t.Fatalf("key %d %s", w.Code, w.Body)
	}
	return k.Key
}

func (f *apiFixture) verifiedDomain(t *testing.T) string {
	t.Helper()
	f.developer(nil)
	domain := f.ws + ".forke.test"
	w := f.request("POST", "/api/domains", `{"name":"`+domain+`"}`, "")
	var identity struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &identity)
	if w.Code != 201 {
		t.Fatalf("domain %d %s", w.Code, w.Body)
	}
	return domain
}

func decodeJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return m
}

func TestResendCompatibleBroadcastFlow(t *testing.T) {
	f := newAPIFixture(t)
	key := f.key(t, "full_access")
	domain := f.verifiedDomain(t)

	w := f.request("POST", "/api/resend/audiences", `{"name":"Forke Subscribers"}`, key)
	if w.Code != 201 {
		t.Fatalf("create audience %d %s", w.Code, w.Body)
	}
	audienceID := decodeJSON(t, w.Body.String())["id"].(string)
	if again := f.request("POST", "/api/resend/audiences", `{"name":"forke subscribers"}`, key); again.Code != 201 || decodeJSON(t, again.Body.String())["id"] != audienceID {
		t.Fatalf("audience not reused: %s", again.Body)
	}
	if list := f.request("GET", "/api/resend/audiences", "", key); !strings.Contains(list.Body.String(), audienceID) {
		t.Fatalf("list %s", list.Body)
	}
	for _, email := range []string{"reader@example.com", "Reader@example.com", "gone@example.com"} {
		if w := f.request("POST", "/api/resend/audiences/"+audienceID+"/contacts", `{"email":"`+email+`","unsubscribed":false}`, key); w.Code != 201 {
			t.Fatalf("contact %s %d %s", email, w.Code, w.Body)
		}
	}
	if w := f.request("POST", "/api/resend/audiences/"+audienceID+"/contacts", `{"email":"gone@example.com","unsubscribed":true}`, key); w.Code != 201 {
		t.Fatalf("unsubscribe contact %d", w.Code)
	}
	if w := f.request("POST", "/api/resend/audiences/00000000-0000-0000-0000-000000000000/contacts", `{"email":"x@example.com"}`, key); w.Code != 404 {
		t.Fatalf("unknown audience %d", w.Code)
	}

	w = f.request("POST", "/api/resend/broadcasts", `{"audience_id":"`+audienceID+`","from":"Forke Blog <blog@`+domain+`>","reply_to":"support@`+domain+`",
		"subject":"New post","html":"<p>Hi</p><a href=\"{{{RESEND_UNSUBSCRIBE_URL}}}\">Unsubscribe</a>","name":"Blog: New post"}`, key)
	if w.Code != 201 {
		t.Fatalf("create broadcast %d %s", w.Code, w.Body)
	}
	broadcastID := decodeJSON(t, w.Body.String())["id"].(string)
	if got := dbtest.QueryString(t, f.pool, `SELECT reply_to || '|' || c.name FROM emails e JOIN campaigns c ON c.id = e.campaign_id WHERE e.id = $1`, broadcastID); got != "support@"+domain+"|Blog: New post" {
		t.Fatalf("stored %q", got)
	}
	if w := f.request("POST", "/api/resend/broadcasts/"+broadcastID+"/send", `{}`, key); w.Code != 200 || decodeJSON(t, w.Body.String())["id"] != broadcastID {
		t.Fatalf("send %d %s", w.Code, w.Body)
	}
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM email_recipients WHERE email_id = $1`, broadcastID); n != "1" {
		t.Fatalf("recipients %s, want only the subscribed reader", n)
	}
}

func TestResendCompatibleEmails(t *testing.T) {
	f := newAPIFixture(t)
	key := f.key(t, "sending_access")
	domain := f.verifiedDomain(t)

	w := f.request("POST", "/api/resend/emails", `{"from":"Forke <hello@`+domain+`>","to":"person@example.com","reply_to":"support@`+domain+`","subject":"Welcome","html":"<p>Hi</p>"}`, key)
	if w.Code != 200 {
		t.Fatalf("send %d %s", w.Code, w.Body)
	}
	id := decodeJSON(t, w.Body.String())["id"].(string)
	got := f.request("GET", "/api/resend/emails/"+id, "", key)
	body := decodeJSON(t, got.Body.String())
	if got.Code != 200 || body["last_event"] != "queued" || body["from"] != "Forke <hello@"+domain+">" {
		t.Fatalf("get %d %s", got.Code, got.Body)
	}

	cases := map[string]string{
		`{"from":"hello@` + domain + `","to":["a@example.com","b@example.com"],"subject":"x","html":"y"}`: "exactly_one_to_address_required",
		`{"from":"not an address","to":"a@example.com","subject":"x","html":"y"}`:                         "invalid_from",
		`{"from":"hello@elsewhere.test","to":"a@example.com","subject":"x","html":"y"}`:                   "",
	}
	for body, message := range cases {
		w := f.request("POST", "/api/resend/emails", body, key)
		resp := decodeJSON(t, w.Body.String())
		if w.Code < 400 || resp["name"] == nil || (message != "" && resp["message"] != message) {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if w := f.request("POST", "/api/resend/audiences", `{"name":"x"}`, key); w.Code != 403 {
		t.Fatalf("sending key created audience: %d", w.Code)
	}
}
