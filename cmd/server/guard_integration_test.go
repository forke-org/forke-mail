// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"encoding/json"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/guard"
)

func TestPlatformGuardHoldsUnprovenProjects(t *testing.T) {
	f := newAPIFixture(t)
	t.Cleanup(func() { dbtest.Exec(t, f.pool, `UPDATE platform_guard SET active = false WHERE id = 1`) })
	domain := f.ws + ".guard.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	dbtest.Exec(t, f.pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'reader@example.com')`, f.ws)
	dbtest.Exec(t, f.pool, `
		INSERT INTO transactional_messages (workspace_id, request_hash, fingerprint, to_address, from_address, subject, text, status, sent_at)
		VALUES ($1, md5(random()::text), md5(random()::text), 'x@example.com', 'me@example.com', 'S', 'x', 'sent', now())`, f.ws)

	g := guard.Guard{Pool: f.pool, MaxBounceRate: -1, MaxComplaintRate: 1, MinSample: 1}
	if err := g.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT active::text || ':' || workspace_held($1)::text FROM platform_guard`, f.ws); got != "true:true" {
		t.Fatalf("guard %s", got)
	}
	w := f.request("POST", "/api/campaigns", `{"name":"Held"}`, "")
	var campaign struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &campaign)
	w = f.request("POST", "/api/campaigns/"+campaign.ID+"/emails", `{"subject":"Hi","from_address":"me@`+domain+`","text":"Hello","audience":{"all":true}}`, "")
	var email struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &email)
	if w := f.request("POST", "/api/emails/"+email.ID+"/send", "", ""); w.Code != 409 {
		t.Fatalf("held project launched %d %s", w.Code, w.Body)
	}
	w = f.request("POST", "/api/transactional", `{"from_address":"me@`+domain+`","to":"user@example.com","subject":"Code","text":"123"}`, "")
	if w.Code != 202 {
		t.Fatalf("transactional blocked by guard %d %s", w.Code, w.Body)
	}
	dbtest.Exec(t, f.pool, `
		INSERT INTO transactional_messages (workspace_id, request_hash, fingerprint, to_address, from_address, subject, text, status, sent_at)
		SELECT $1, md5(random()::text || g), md5(random()::text || g), 'x' || g || '@example.com', 'me@example.com', 'S', 'x', 'sent', now() - interval '30 days'
		FROM generate_series(1, 5000) g`, f.ws)
	if got := dbtest.QueryString(t, f.pool, `SELECT workspace_held($1)::text`, f.ws); got != "false" {
		t.Fatalf("proven project still held: %s", got)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT COUNT(*)::text FROM workspace_alerts WHERE kind = 'platform_guard'`); got == "0" && dbtest.QueryString(t, f.pool, `SELECT COUNT(*)::text FROM workspaces WHERE platform`) != "0" {
		t.Fatal("no admin alert")
	}
	if err := g.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, f.pool, `UPDATE platform_guard SET active = false WHERE id = 1`)
	dbtest.Exec(t, f.pool, `UPDATE workspaces SET warmup_until = now() + interval '1 day' WHERE id = $1`, f.ws)
	if got := dbtest.QueryString(t, f.pool, `SELECT workspace_held($1)::text`, f.ws); got != "false" {
		t.Fatalf("held while guard is off: %s", got)
	}
}
