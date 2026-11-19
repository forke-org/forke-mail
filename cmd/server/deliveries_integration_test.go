package main

import (
	"context"
	"testing"
)

func TestDeliveriesSearch(t *testing.T) {
	f := newAPIFixture(t)
	insert := func(to, status string, bounced bool) {
		_, err := f.pool.Exec(context.Background(), `
			INSERT INTO transactional_messages (workspace_id, request_hash, fingerprint, to_address, from_address, subject, html, text, status, sent_at, bounced_at, bounce_type, last_error)
			VALUES ($1, md5(random()::text), md5(random()::text), $2, 'app@example.com', 'Receipt', '<p>hi</p>', 'hi', $3, now(),
				CASE WHEN $4 THEN now() END, CASE WHEN $4 THEN 'Permanent' END, CASE WHEN $3 = 'failed' THEN 'message_rejected: blocked' END)`,
			f.ws, to, status, bounced)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("ok@example.com", "sent", false)
	insert("gone@example.com", "sent", true)
	insert("blocked@example.com", "failed", false)

	cases := map[string]int{
		"/api/deliveries":                        3,
		"/api/deliveries?status=problems":        2,
		"/api/deliveries?status=bounced":         1,
		"/api/deliveries?q=BLOCKED":              1,
		"/api/deliveries?q=%25":                  0,
		"/api/deliveries?kind=broadcast":         0,
		"/api/deliveries?kind=transactional&q=o": 3,
	}
	for path, want := range cases {
		w := f.request("GET", path, "", "")
		if w.Code != 200 {
			t.Fatalf("%s status %d %s", path, w.Code, w.Body)
		}
		body := decodeJSON(t, w.Body.String())
		if got := int(body["total"].(float64)); got != want {
			t.Fatalf("%s total %d, want %d: %s", path, got, want, w.Body)
		}
	}
	if w := f.request("GET", "/api/deliveries?status=nope", "", ""); w.Code != 400 {
		t.Fatalf("invalid status %d", w.Code)
	}
	other := newAPIFixture(t)
	if body := decodeJSON(t, other.request("GET", "/api/deliveries", "", "").Body.String()); body["total"].(float64) != 0 {
		t.Fatal("deliveries leaked across workspaces")
	}
}

func TestAlertSettings(t *testing.T) {
	f := newAPIFixture(t)
	if w := f.request("PUT", "/api/settings/alerts", `{"emails":false,"slack_url":"https://evil.example.com/hook"}`, ""); w.Code != 422 {
		t.Fatalf("bad slack url %d", w.Code)
	}
	w := f.request("PUT", "/api/settings/alerts", `{"emails":false,"slack_url":"https://hooks.slack.com/services/T/B/x"}`, "")
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body)
	}
	body := decodeJSON(t, f.request("GET", "/api/settings/alerts", "", "").Body.String())
	if body["emails"] != false || body["slack_url"] != "https://hooks.slack.com/services/T/B/x" || body["thresholds"] == nil {
		t.Fatalf("settings %v", body)
	}
}
