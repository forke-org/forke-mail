// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestTemplatesCrudAndSend(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	domain := f.ws + ".templates.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	w := f.request("POST", "/api/templates", `{"name":"Reset","subject":"Hi {{ name }}","html":"<p>Code {{code}} for {{name}}</p>","text":"Code {{code}}"}`, "")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body)
	}
	var tpl struct {
		ID        string
		Variables []string
	}
	json.Unmarshal(w.Body.Bytes(), &tpl)
	if strings.Join(tpl.Variables, ",") != "code,name" {
		t.Fatalf("variables %v", tpl.Variables)
	}
	if w := f.request("POST", "/api/templates", `{"name":"reset","subject":"x","text":"y"}`, ""); w.Code != 409 {
		t.Fatalf("duplicate name %d", w.Code)
	}
	w = f.request("POST", "/api/transactional", `{"from_address":"hi@`+domain+`","to":"user@example.com","template_id":"`+tpl.ID+`","data":{"name":"<Ada>","code":"482913"}}`, "")
	if w.Code != 202 {
		t.Fatalf("send %d %s", w.Code, w.Body)
	}
	var msg struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &msg)
	got := dbtest.QueryString(t, f.pool, `SELECT subject || '|' || html FROM transactional_messages WHERE id = $1`, msg.ID)
	if got != "Hi <Ada>|<p>Code 482913 for &lt;Ada&gt;</p>" {
		t.Fatalf("rendered %q", got)
	}
	if w := f.request("POST", "/api/transactional", `{"from_address":"hi@`+domain+`","to":"user@example.com","template_id":"00000000-0000-0000-0000-000000000000"}`, ""); w.Code != 404 {
		t.Fatalf("missing template %d", w.Code)
	}
	other := newAPIFixture(t)
	if w := other.request("GET", "/api/templates/"+tpl.ID, "", ""); w.Code != 404 {
		t.Fatalf("cross-workspace read %d", w.Code)
	}
	if w := f.request("DELETE", "/api/templates/"+tpl.ID, "", ""); w.Code != 204 {
		t.Fatalf("delete %d", w.Code)
	}
}
