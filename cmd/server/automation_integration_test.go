package main

import (
	"encoding/json"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"
	"forkemail/internal/workflows"
)

func TestSegmentsEventsAndWorkflows(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	domain := f.ws + ".auto.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)

	w := f.request("POST", "/api/events", `{"email":"Pro@Example.com","event":"user.signed-up","data":{"plan":"pro"},"contact":{"first_name":"Ada","data":{"plan":"pro","seats":5}}}`, "")
	if w.Code != 202 || !strings.Contains(w.Body.String(), `"contact_created":true`) {
		t.Fatalf("track %d %s", w.Code, w.Body)
	}
	f.request("POST", "/api/events", `{"email":"free@example.com","event":"user.signed-up","contact":{"data":{"plan":"free","seats":1}}}`, "")
	if w := f.request("POST", "/api/events", `{"email":"x@example.com","event":"bad event!"}`, ""); w.Code != 422 {
		t.Fatalf("bad event name %d", w.Code)
	}

	w = f.request("POST", "/api/segments/preview", `{"match":"all","rules":[{"field":"data.seats","op":"greater_than","value":"2"},{"field":"event","op":"performed","value":"user.signed-up","days":7}]}`, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"size":1`) {
		t.Fatalf("preview %d %s", w.Code, w.Body)
	}
	if w := f.request("POST", "/api/segments/preview", `{"rules":[{"field":"data.x; drop table contacts","op":"equals","value":"1"}]}`, ""); w.Code != 422 {
		t.Fatalf("unsafe field accepted %d", w.Code)
	}
	w = f.request("POST", "/api/segments", `{"name":"Pro","match":"any","rules":[{"field":"data.plan","op":"equals","value":"PRO"}]}`, "")
	var seg struct {
		ID   string
		Size int
	}
	json.Unmarshal(w.Body.Bytes(), &seg)
	if w.Code != 201 || seg.Size != 1 {
		t.Fatalf("segment %d %s", w.Code, w.Body)
	}

	w = f.request("POST", "/api/templates", `{"name":"Welcome","subject":"Welcome {{first_name}}","html":"<p>Plan {{plan}}</p>"}`, "")
	var tpl struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &tpl)
	body := `{"name":"Onboarding","trigger_event":"trial.started","enabled":true,"steps":[{"type":"wait","minutes":1},{"type":"send","template_id":"` + tpl.ID + `","from_address":"hi@` + domain + `"}]}`
	w = f.request("POST", "/api/workflows", body, "")
	var wf struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &wf)
	if w.Code != 201 {
		t.Fatalf("workflow %d %s", w.Code, w.Body)
	}
	if w := f.request("POST", "/api/workflows", strings.Replace(body, "hi@"+domain, "hi@unverified.test", 1), ""); w.Code != 422 {
		t.Fatalf("unverified from accepted %d", w.Code)
	}
	w = f.request("POST", "/api/events", `{"email":"pro@example.com","event":"trial.started"}`, "")
	if !strings.Contains(w.Body.String(), `"workflows_started":1`) {
		t.Fatalf("workflow not started %s", w.Body)
	}
	w = f.request("POST", "/api/events", `{"email":"pro@example.com","event":"trial.started"}`, "")
	if !strings.Contains(w.Body.String(), `"workflows_started":0`) {
		t.Fatalf("re-entered %s", w.Body)
	}
	dbtest.Exec(t, f.pool, `UPDATE workflow_runs SET next_run_at = now() WHERE workflow_id = $1`, wf.ID)
	runner := workflows.Runner{Pool: f.pool, Mail: transactional.Service{Pool: f.pool}}
	for i := 0; i < 3; i++ {
		if _, err := runner.Once(t.Context()); err != nil {
			t.Fatal(err)
		}
		dbtest.Exec(t, f.pool, `UPDATE workflow_runs SET next_run_at = now() WHERE workflow_id = $1`, wf.ID)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT status FROM workflow_runs WHERE workflow_id = $1`, wf.ID); got != "done" {
		t.Fatalf("run %s", got)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT subject || '|' || html FROM transactional_messages WHERE workspace_id = $1 AND to_address = 'pro@example.com'`, f.ws); got != "Welcome Ada|<p>Plan pro</p>" {
		t.Fatalf("sent %q", got)
	}

	w = f.request("GET", "/api/data", "", "")
	if !strings.Contains(w.Body.String(), `"segment: Pro"`) || !strings.Contains(w.Body.String(), `"workflow: Onboarding"`) {
		t.Fatalf("data usage %s", w.Body)
	}
	if w := f.request("DELETE", "/api/data/fields/plan", "", ""); w.Code != 409 {
		t.Fatalf("deleted field in use %d", w.Code)
	}
	if w := f.request("DELETE", "/api/data/fields/seats", "", ""); w.Code != 204 {
		t.Fatalf("delete field %d", w.Code)
	}
	if w := f.request("DELETE", "/api/workflows/"+wf.ID, "", ""); w.Code != 204 {
		t.Fatalf("delete workflow %d", w.Code)
	}
	if w := f.request("DELETE", "/api/data/events/trial.started", "", ""); w.Code != 204 {
		t.Fatalf("delete event %d", w.Code)
	}
}

func TestBroadcastToSegment(t *testing.T) {
	f := newAPIFixture(t)
	domain := f.ws + ".segment.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	for _, e := range []string{"a@vip.test", "b@vip.test", "c@other.test"} {
		f.request("POST", "/api/events", `{"email":"`+e+`","event":"seen"}`, "")
	}
	w := f.request("POST", "/api/segments", `{"name":"VIP","rules":[{"field":"email","op":"contains","value":"@vip.test"}]}`, "")
	var seg struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &seg)
	w = f.request("POST", "/api/campaigns", `{"name":"Segmented"}`, "")
	var campaign struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &campaign)
	w = f.request("POST", "/api/campaigns/"+campaign.ID+"/emails", `{"subject":"Hi","from_address":"me@`+domain+`","text":"Hello","audience":{"segment_ids":["`+seg.ID+`"]}}`, "")
	var email struct {
		ID       string
		Audience struct {
			Size       int
			SegmentIDs []string `json:"segment_ids"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &email)
	if w.Code != 201 || email.Audience.Size != 2 || len(email.Audience.SegmentIDs) != 1 {
		t.Fatalf("email %d %s", w.Code, w.Body)
	}
	f.request("POST", "/api/events", `{"email":"d@vip.test","event":"seen"}`, "")
	json.Unmarshal(f.request("GET", "/api/emails/"+email.ID, "", "").Body.Bytes(), &email)
	if email.Audience.Size != 3 {
		t.Fatalf("segment not refreshed: %d", email.Audience.Size)
	}
}

func TestGlobalBounceBlocksEveryProject(t *testing.T) {
	f := newAPIFixture(t)
	domain := f.ws + ".gb.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	address := "dead-" + f.ws[:8] + "@example.com"
	dbtest.Exec(t, f.pool, `INSERT INTO global_bounces (email) VALUES ($1)`, address)
	t.Cleanup(func() { dbtest.Exec(t, f.pool, `DELETE FROM global_bounces WHERE email = $1`, address) })
	w := f.request("POST", "/api/transactional", `{"from_address":"me@`+domain+`","to":"`+strings.ToUpper(address)+`","subject":"S","text":"x"}`, "")
	if w.Code == 202 || !strings.Contains(w.Body.String(), "recipient_suppressed") {
		t.Fatalf("sent to a globally bounced address %d %s", w.Code, w.Body)
	}
	f.request("POST", "/api/events", `{"email":"`+address+`","event":"seen"}`, "")
	f.request("POST", "/api/events", `{"email":"alive-`+f.ws[:8]+`@example.com","event":"seen"}`, "")
	w = f.request("POST", "/api/campaigns", `{"name":"GB"}`, "")
	var campaign struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &campaign)
	w = f.request("POST", "/api/campaigns/"+campaign.ID+"/emails", `{"subject":"Hi","from_address":"me@`+domain+`","text":"Hello","audience":{"all":true}}`, "")
	var email struct{ Audience struct{ Size int } }
	json.Unmarshal(w.Body.Bytes(), &email)
	if email.Audience.Size != 1 {
		t.Fatalf("audience %d", email.Audience.Size)
	}
}

func TestWorkflowExitEvent(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	domain := f.ws + ".exit.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	w := f.request("POST", "/api/templates", `{"name":"Reminder","subject":"Last chance","text":"Tomorrow"}`, "")
	var tpl struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &tpl)
	body := `{"name":"Deletion","trigger_event":"account.deletion_scheduled","exit_event":"account.deletion_cancelled","enabled":true,"steps":[{"type":"wait","minutes":1},{"type":"send","template_id":"` + tpl.ID + `","from_address":"hi@` + domain + `"}]}`
	w = f.request("POST", "/api/workflows", body, "")
	var wf struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &wf)
	if w.Code != 201 {
		t.Fatalf("workflow %d %s", w.Code, w.Body)
	}
	f.request("POST", "/api/events", `{"email":"leaving@example.com","event":"account.deletion_scheduled"}`, "")
	f.request("POST", "/api/events", `{"email":"staying@example.com","event":"account.deletion_scheduled"}`, "")
	dbtest.Exec(t, f.pool, `UPDATE workflow_runs SET created_at = now() - interval '1 minute' WHERE workflow_id = $1`, wf.ID)
	f.request("POST", "/api/events", `{"email":"staying@example.com","event":"account.deletion_cancelled"}`, "")
	runner := workflows.Runner{Pool: f.pool, Mail: transactional.Service{Pool: f.pool}}
	for i := 0; i < 3; i++ {
		dbtest.Exec(t, f.pool, `UPDATE workflow_runs SET next_run_at = now() WHERE workflow_id = $1`, wf.ID)
		if _, err := runner.Once(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	got := dbtest.QueryString(t, f.pool, `SELECT string_agg(c.email || '=' || COALESCE(r.last_error, 'sent'), ',' ORDER BY c.email) FROM workflow_runs r JOIN contacts c ON c.id = r.contact_id WHERE r.workflow_id = $1`, wf.ID)
	if got != "leaving@example.com=sent,staying@example.com=exit_event" {
		t.Fatalf("runs %s", got)
	}
}

func TestWorkflowRespectsTemplateKind(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	domain := f.ws + ".kind.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	ids := map[string]string{}
	for _, kind := range []string{"transactional", "marketing"} {
		w := f.request("POST", "/api/templates", `{"name":"`+kind+`","kind":"`+kind+`","subject":"S {{date}}","text":"x"}`, "")
		var tpl struct{ ID string }
		json.Unmarshal(w.Body.Bytes(), &tpl)
		ids[kind] = tpl.ID
		f.request("POST", "/api/workflows", `{"name":"`+kind+`","trigger_event":"go.`+kind+`","enabled":true,"steps":[{"type":"send","template_id":"`+tpl.ID+`","from_address":"hi@`+domain+`"}]}`, "")
	}
	f.request("POST", "/api/events", `{"email":"gone@example.com","event":"seen"}`, "")
	dbtest.Exec(t, f.pool, `UPDATE contacts SET status = 'unsubscribed' WHERE workspace_id = $1 AND email = 'gone@example.com'`, f.ws)
	f.request("POST", "/api/events", `{"email":"gone@example.com","event":"go.transactional"}`, "")
	f.request("POST", "/api/events", `{"email":"gone@example.com","event":"go.marketing"}`, "")
	runner := workflows.Runner{Pool: f.pool, Mail: transactional.Service{Pool: f.pool}}
	for i := 0; i < 2; i++ {
		if _, err := runner.Once(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	got := dbtest.QueryString(t, f.pool, `
		SELECT string_agg(w.name || '=' || r.status || ':' || COALESCE(r.last_error, ''), ',' ORDER BY w.name)
		FROM workflow_runs r JOIN workflows w ON w.id = r.workflow_id WHERE r.workspace_id = $1`, f.ws)
	if got != "marketing=cancelled:contact_unsubscribed,transactional=done:" {
		t.Fatalf("runs %s", got)
	}
	if subject := dbtest.QueryString(t, f.pool, `SELECT subject FROM transactional_messages WHERE workspace_id = $1`, f.ws); strings.Contains(subject, "{{date}}") {
		t.Fatalf("date not filled: %s", subject)
	}
}
