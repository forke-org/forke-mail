package main

import (
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"

	"forkemail/internal/config"
	"forkemail/internal/db/dbtest"
)

func fakeSpamd(t *testing.T, score string) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			io.ReadAll(c)
			c.Write([]byte("SPAMD/1.1 0 EX_OK\r\nSpam: True ; " + score + " / 5.0\r\n\r\n---- ---\n 6.0 FREE_MONEY             Promises free money\n"))
			c.Close()
		}
	}()
	return l.Addr().String()
}

func TestSpammyBroadcastIsBlocked(t *testing.T) {
	addr := fakeSpamd(t, "6.0")
	f, _ := newIntegratedFixture(t, func(c *config.Config) { c.SpamdAddr = addr; c.SpamBlockScore = 5 })
	domain := f.ws + ".spam.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	dbtest.Exec(t, f.pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'reader@example.com')`, f.ws)
	w := f.request("POST", "/api/campaigns", `{"name":"Spam"}`, "")
	var campaign struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &campaign)
	w = f.request("POST", "/api/campaigns/"+campaign.ID+"/emails", `{"subject":"FREE MONEY!!","from_address":"me@`+domain+`","html":"<a href=\"https://bit.ly/x\">claim</a>","audience":{"all":true}}`, "")
	var email struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &email)
	w = f.request("GET", "/api/emails/"+email.ID+"/spam-check", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"spam":true`) || !strings.Contains(w.Body.String(), "Promises free money") || !strings.Contains(w.Body.String(), "link shortener") {
		t.Fatalf("check %d %s", w.Code, w.Body)
	}
	w = f.request("POST", "/api/emails/"+email.ID+"/send", "", "")
	if w.Code != 422 || !strings.Contains(w.Body.String(), "content_flagged") {
		t.Fatalf("spammy broadcast sent %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT status FROM emails WHERE id = $1`, email.ID); got != "draft" {
		t.Fatalf("status %s", got)
	}
}
