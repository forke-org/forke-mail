package main

import (
	"context"
	"encoding/json"
	"testing"

	"forkemail/internal/accounts"
	"forkemail/internal/config"
	"forkemail/internal/db/dbtest"

	"github.com/jackc/pgx/v5"
)

type projectList struct {
	Projects []struct {
		ID, Name, Role string
		Current        bool
	}
}

func TestProjectsCreateSwitchAndShare(t *testing.T) {
	pool := dbtest.Pool(t)
	sender := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, sender, sender+".projects.test")
	f, _ := newIntegratedFixture(t, func(c *config.Config) {
		c.AlertWorkspaceID = sender
		c.InviteFrom = "hello@" + sender + ".projects.test"
		c.MaxProjects = 2
		c.ProjectSendLimit = 250
	})
	w := f.request("POST", "/api/projects", `{"name":"Side project"}`, "")
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body)
	}
	var created struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &created)
	t.Cleanup(func() { dbtest.Exec(t, pool, `DELETE FROM workspaces WHERE id = $1`, created.ID) })

	var list projectList
	json.Unmarshal(f.request("GET", "/api/projects", "", "").Body.Bytes(), &list)
	if len(list.Projects) != 2 {
		t.Fatalf("projects %+v", list)
	}
	for _, p := range list.Projects {
		if p.ID == created.ID && (!p.Current || p.Role != "super_admin") {
			t.Fatalf("new project not current: %+v", p)
		}
	}
	var me struct{ Workspace struct{ ID string } }
	json.Unmarshal(f.request("GET", "/api/me", "", "").Body.Bytes(), &me)
	if me.Workspace.ID != created.ID {
		t.Fatalf("session still on %s", me.Workspace.ID)
	}
	if got := dbtest.QueryString(t, pool, `SELECT plan || ':' || trial_send_limit FROM workspaces WHERE id = $1`, created.ID); got != "trial:250" {
		t.Fatalf("defaults %s", got)
	}
	if w := f.request("POST", "/api/projects", `{"name":"Third"}`, ""); w.Code != 409 {
		t.Fatalf("limit not enforced %d", w.Code)
	}
	if w := f.request("POST", "/api/projects/switch", `{"id":"`+sender+`"}`, ""); w.Code != 404 {
		t.Fatalf("switched into a foreign project %d", w.Code)
	}
	if w := f.request("POST", "/api/projects/switch", `{"id":"`+f.ws+`"}`, ""); w.Code != 204 {
		t.Fatalf("switch back %d", w.Code)
	}

	other := dbtest.Workspace(t, pool, "trial", 100)
	var otherUser string
	otherEmail := "other-" + other[:8] + "@example.com"
	pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		var err error
		otherUser, err = accounts.CreateUser(context.Background(), tx, other, "Other", otherEmail, "long-enough-password")
		return err
	})
	if w := f.request("POST", "/api/team/invites", `{"email":"`+otherEmail+`","role":"admin"}`, ""); w.Code != 201 {
		t.Fatalf("add existing %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM workspace_members WHERE user_id = $1`, otherUser); got != "2" {
		t.Fatalf("memberships %s", got)
	}
	if w := f.request("POST", "/api/team/invites", `{"email":"`+otherEmail+`","role":"admin"}`, ""); w.Code != 409 {
		t.Fatalf("duplicate member %d", w.Code)
	}
	if w := f.request("DELETE", "/api/team/members/"+otherUser, "", ""); w.Code != 204 {
		t.Fatalf("remove %d", w.Code)
	}
	if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text || ':' || (SELECT workspace_id::text FROM users WHERE id = $1) FROM workspace_members WHERE user_id = $1`, otherUser); got != "1:"+other {
		t.Fatalf("after remove %s", got)
	}
}

func TestRemovingHomeProjectRehomesUser(t *testing.T) {
	pool := dbtest.Pool(t)
	home := dbtest.Workspace(t, pool, "trial", 100)
	second := dbtest.Workspace(t, pool, "trial", 100)
	ctx := context.Background()
	var user string
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var err error
		if user, err = accounts.CreateUser(ctx, tx, home, "Two", "two-"+home[:8]+"@example.com", "long-enough-password"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'admin')`, second, user); err != nil {
			return err
		}
		_, err = accounts.DeleteWorkspace(ctx, tx, home)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := dbtest.QueryString(t, pool, `SELECT workspace_id::text || ':' || role FROM users WHERE id = $1`, user); got != second+":admin" {
		t.Fatalf("user = %s", got)
	}
}
