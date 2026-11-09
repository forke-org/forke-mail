package main

import (
	"encoding/json"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/sender"
	"testing"
)

func TestBothWorkersShareTrialQuota(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	dbtest.Exec(t, f.pool, `UPDATE workspaces SET plan='trial',trial_send_limit=2 WHERE id=$1`, f.ws)
	f.worker.SetLimits(sender.Limits{Max24HourSend: 100, SentLast24Hours: 90})
	domain := f.ws + ".shared.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	w := f.request("POST", "/api/transactional", `{"from_address":"hello@`+domain+`","to":"transactional@example.com","subject":"Shared quota","text":"Hi"}`, "")
	var msg struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &msg)
	if w.Code != 202 {
		t.Fatal("transactional acceptance failed")
	}
	dbtest.Exec(t, f.pool, `INSERT INTO contacts(workspace_id,email) VALUES($1,'broadcast@example.com')`, f.ws)
	email := dbtest.DraftEmail(t, f.pool, f.ws)
	dbtest.Exec(t, f.pool, `UPDATE emails SET from_address=$1,audience_all=true WHERE id=$2 AND workspace_id=$3`, "hello@"+domain, email, f.ws)
	if w = f.request("POST", "/api/emails/"+email+"/send", "", ""); w.Code != 200 {
		t.Fatalf("broadcast launch %d %s", w.Code, w.Body.String())
	}
	f.worker.SetLimits(sender.Limits{Max24HourSend: 1000})
	eventually(t, func() bool {
		return dbtest.QueryString(t, f.pool, `SELECT ((SELECT count(delivered_at) FROM email_recipients WHERE workspace_id=$1)+(SELECT count(delivered_at) FROM transactional_messages WHERE workspace_id=$1))::text`, f.ws) == "2"
	})
	if dbtest.QueryString(t, f.pool, `SELECT trial_sent_count::text FROM workspaces WHERE id=$1`, f.ws) != "2" {
		t.Fatal("shared quota not charged exactly twice")
	}
	if dbtest.QueryString(t, f.pool, `SELECT count(delivered_at)::text FROM transactional_messages WHERE workspace_id=$1 AND id=$2`, f.ws, msg.ID) != "1" {
		t.Fatal("transactional worker failed")
	}
	if dbtest.QueryString(t, f.pool, `SELECT count(delivered_at)::text FROM email_recipients WHERE workspace_id=$1 AND email_id=$2`, f.ws, email) != "1" {
		t.Fatal("broadcast worker failed")
	}
	if w = f.request("POST", "/api/transactional", `{"from_address":"hello@`+domain+`","to":"third@example.com","subject":"Over quota","text":"Hi"}`, ""); w.Code != 402 {
		t.Fatalf("quota exceeded accepted: %d", w.Code)
	}
	if dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM transactional_messages WHERE workspace_id=$1`, f.ws) != "1" {
		t.Fatal("rejected request persisted")
	}
}
