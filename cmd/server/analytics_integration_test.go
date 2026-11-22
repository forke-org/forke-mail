package main

import (
	"encoding/json"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestAnalytics(t *testing.T) {
	f := newAPIFixture(t)
	insert := func(age string, delivered, bounced bool) {
		dbtest.Exec(t, f.pool, `
			INSERT INTO transactional_messages (workspace_id, request_hash, fingerprint, to_address, from_address, subject, text, status, sent_at, delivered_at, bounced_at, bounce_type)
			VALUES ($1, md5(random()::text), md5(random()::text), 'a@example.com', 'me@example.com', 'S', 'x', 'sent', now() - $2::interval,
				CASE WHEN $3 THEN now() - $2::interval END, CASE WHEN $4 THEN now() - $2::interval END, CASE WHEN $4 THEN 'Permanent' END)`,
			f.ws, age, delivered, bounced)
	}
	insert("1 hour", true, false)
	insert("2 days", true, false)
	insert("3 days", false, true)
	insert("40 days", true, false)

	var out struct {
		Current, Previous struct{ Sent, Delivered, Bounced int }
		Daily             []struct {
			Date                           string
			Sent, Transactional, Broadcast int
		}
		Streams []struct {
			Stream string
			Sent   int
		}
	}
	w := f.request("GET", "/api/analytics?days=30", "", "")
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Current.Sent != 3 || out.Current.Delivered != 2 || out.Current.Bounced != 1 || out.Previous.Sent != 1 {
		t.Fatalf("totals %+v %+v", out.Current, out.Previous)
	}
	total, tx := 0, 0
	for _, d := range out.Daily {
		total += d.Sent
		tx += d.Transactional
		if d.Broadcast+d.Transactional != d.Sent {
			t.Fatalf("split does not add up %+v", d)
		}
	}
	if len(out.Daily) != 30 || total != 3 || tx != 3 {
		t.Fatalf("daily %d days, %d sent", len(out.Daily), total)
	}
	if len(out.Streams) != 1 || out.Streams[0].Stream != "transactional" {
		t.Fatalf("streams %+v", out.Streams)
	}
	json.Unmarshal(f.request("GET", "/api/analytics?days=7&stream=broadcast", "", "").Body.Bytes(), &out)
	if out.Current.Sent != 0 {
		t.Fatalf("broadcast filter %+v", out.Current)
	}
	if w := f.request("GET", "/api/analytics?days=12", "", ""); w.Code != 400 {
		t.Fatalf("bad period %d", w.Code)
	}
}
