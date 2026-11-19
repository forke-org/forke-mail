package main

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"forkemail/internal/config"
	"forkemail/internal/db/dbtest"
)

func TestTeamInviteJoinAndRemove(t *testing.T) {
	pool := dbtest.Pool(t)
	sender := dbtest.Workspace(t, pool, "paid", 0)
	domain := sender + ".team.test"
	dbtest.Exec(t, pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, sender, domain)
	f, _ := newIntegratedFixture(t, func(c *config.Config) {
		c.AlertWorkspaceID = sender
		c.InviteFrom = "hello@" + domain
	})
	mate := "mate-" + f.ws[:8] + "@example.com"
	w := f.request("POST", "/api/team/invites", `{"email":"`+mate+`","role":"admin"}`, "")
	if w.Code != 201 {
		t.Fatalf("invite %d %s", w.Code, w.Body)
	}
	var link string
	eventually(t, func() bool {
		link = dbtest.QueryString(t, pool, `SELECT COALESCE(MAX(payload->'message'->>'text'), '') FROM message_logs WHERE workspace_id=$1`, sender)
		return strings.Contains(link, "team=")
	})
	token := regexp.MustCompile(`team=([A-Za-z0-9_-]+)`).FindStringSubmatch(link)[1]

	lookup := httptest.NewRecorder()
	f.router.ServeHTTP(lookup, httptest.NewRequest("GET", "/api/team/join?token="+token, nil))
	if lookup.Code != 200 || !strings.Contains(lookup.Body.String(), mate) {
		t.Fatalf("lookup %d %s", lookup.Code, lookup.Body)
	}
	join := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/team/join", strings.NewReader(`{"token":"`+token+`","name":"Mate","username":"mate`+f.ws[:8]+`","password":"long-enough-password"}`))
	req.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(join, req)
	if join.Code != 201 {
		t.Fatalf("join %d %s", join.Code, join.Body)
	}
	var list struct {
		Members []struct {
			ID, Email, Role string
			You             bool
		}
		Invites []any
	}
	json.Unmarshal(f.request("GET", "/api/team", "", "").Body.Bytes(), &list)
	if len(list.Members) != 2 || len(list.Invites) != 0 {
		t.Fatalf("team %+v", list)
	}
	var mateID string
	for _, m := range list.Members {
		if m.Email == mate {
			mateID = m.ID
			if m.Role != "admin" {
				t.Fatalf("role %s", m.Role)
			}
		}
	}
	if w := f.request("DELETE", "/api/team/members/"+mateID, "", ""); w.Code != 204 {
		t.Fatalf("remove %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM users WHERE workspace_id=$1`, f.ws); got != "1" {
		t.Fatalf("members left %s", got)
	}
}
