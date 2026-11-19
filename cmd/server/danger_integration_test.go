package main

import (
	"context"
	"encoding/json"
	"testing"

	"forkemail/internal/accounts"
	"forkemail/internal/db/dbtest"

	"github.com/jackc/pgx/v5"
)

func TestProjectDangerZone(t *testing.T) {
	f := newAPIFixture(t)
	ctx := context.Background()
	name := dbtest.QueryString(t, f.pool, `SELECT name FROM workspaces WHERE id = $1`, f.ws)
	dbtest.Exec(t, f.pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'a@example.com')`, f.ws)
	dbtest.Exec(t, f.pool, `INSERT INTO templates (workspace_id, name, subject, text) VALUES ($1, 'T', 'S', 'x')`, f.ws)

	if w := f.request("POST", "/api/projects/current/reset", `{"confirm":"wrong"}`, ""); w.Code != 422 {
		t.Fatalf("reset without confirm %d", w.Code)
	}
	if w := f.request("POST", "/api/projects/current/reset", `{"confirm":"`+name+`"}`, ""); w.Code != 204 {
		t.Fatalf("reset %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT (SELECT COUNT(*) FROM contacts WHERE workspace_id=$1) + (SELECT COUNT(*) FROM templates WHERE workspace_id=$1)`, f.ws); got != "0" {
		t.Fatalf("reset left %s rows", got)
	}
	if w := f.request("POST", "/api/projects/current/delete", `{"confirm":"`+name+`"}`, ""); w.Code != 409 {
		t.Fatalf("deleting only project %d", w.Code)
	}
	if w := f.request("POST", "/api/projects/current/leave", `{}`, ""); w.Code != 409 {
		t.Fatalf("leaving only project %d", w.Code)
	}

	w := f.request("POST", "/api/projects", `{"name":"Second"}`, "")
	var second struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &second)
	t.Cleanup(func() { dbtest.Exec(t, f.pool, `DELETE FROM workspaces WHERE id = $1`, second.ID) })
	if w := f.request("PATCH", "/api/projects/current", `{"name":"Renamed"}`, ""); w.Code != 200 {
		t.Fatalf("rename %d", w.Code)
	}
	mate := dbtest.Workspace(t, f.pool, "trial", 10)
	var mateID string
	pgx.BeginFunc(ctx, f.pool, func(tx pgx.Tx) error {
		var err error
		mateID, err = accounts.CreateUser(ctx, tx, mate, "Mate", "mate-"+mate[:8]+"@example.com", "long-enough-password")
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'admin')`, second.ID, mateID)
		return err
	})
	if w := f.request("PATCH", "/api/team/members/"+mateID, `{"role":"super_admin"}`, ""); w.Code != 204 {
		t.Fatalf("promote %d %s", w.Code, w.Body)
	}
	if w := f.request("POST", "/api/projects/current/delete", `{"confirm":"Renamed"}`, ""); w.Code != 204 {
		t.Fatalf("delete %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT (SELECT COUNT(*) FROM workspaces WHERE id = $1)::text || ':' || (SELECT COUNT(*) FROM users WHERE id = $2)`, second.ID, mateID); got != "0:1" {
		t.Fatalf("after delete project:mate = %s", got)
	}
	var me struct{ Workspace struct{ ID string } }
	json.Unmarshal(f.request("GET", "/api/me", "", "").Body.Bytes(), &me)
	if me.Workspace.ID != f.ws {
		t.Fatalf("session not moved back home: %s", me.Workspace.ID)
	}
}

func TestDeleteAccount(t *testing.T) {
	f := newAPIFixture(t)
	if w := f.request("POST", "/api/account/delete", `{"password":"nope"}`, ""); w.Code != 401 {
		t.Fatalf("wrong password %d", w.Code)
	}
	if w := f.request("PATCH", "/api/account", `{"name":"New Name","username":"newname`+f.ws[:6]+`"}`, ""); w.Code != 204 {
		t.Fatalf("update %d %s", w.Code, w.Body)
	}
	if w := f.request("POST", "/api/account/password", `{"current":"test-password","new":"another-long-password"}`, ""); w.Code != 204 {
		t.Fatalf("password %d %s", w.Code, w.Body)
	}
	if w := f.request("POST", "/api/account/delete", `{"password":"another-long-password"}`, ""); w.Code != 204 {
		t.Fatalf("delete %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT (SELECT COUNT(*) FROM workspaces WHERE id = $1) + (SELECT COUNT(*) FROM users WHERE workspace_id = $1)`, f.ws); got != "0" {
		t.Fatalf("leftovers %s", got)
	}
	if w := f.request("GET", "/api/me", "", ""); w.Code != 401 {
		t.Fatalf("session survived %d", w.Code)
	}
}
