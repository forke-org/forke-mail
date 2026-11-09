package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/forke"
	"forkemail/internal/ratelimit"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type forkeFixture struct {
	t      *testing.T
	admins *pgxpool.Pool
	router http.Handler
	suffix string
}

func newForkeFixture(t *testing.T) *forkeFixture {
	t.Helper()
	pool := dbtest.Pool(t)
	suffix := strings.ReplaceAll(dbtest.QueryString(t, pool, `SELECT gen_random_uuid()::text`), "-", "")
	schema := "forke_" + suffix
	dbtest.Exec(t, pool, `CREATE SCHEMA `+schema)
	dbtest.Exec(t, pool, `CREATE TABLE `+schema+`.admins (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL UNIQUE, name text NOT NULL, username text UNIQUE,
		password_hash text, role text NOT NULL DEFAULT 'admin', is_disabled boolean NOT NULL DEFAULT false)`)
	t.Cleanup(func() {
		dbtest.Exec(t, pool, `DELETE FROM users WHERE email LIKE '%@`+suffix+`.forke.test'`)
		dbtest.Exec(t, pool, `DROP SCHEMA `+schema+` CASCADE`)
	})
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	admins, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admins.Close)
	config := testConfig("production")
	config.ForkeWorkspace = "Forke " + suffix
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	svc := services{sendRate: func() float64 { return 20 }, admins: &forke.Directory{Pool: admins}}
	t.Cleanup(func() { dbtest.Exec(t, pool, `DELETE FROM workspaces WHERE name = $1`, config.ForkeWorkspace) })
	return &forkeFixture{t: t, admins: admins, router: newRouter(config, pool, lim, svc), suffix: suffix}
}

func (f *forkeFixture) addAdmin(name, role, password string) string {
	f.t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), 4)
	email := name + "@" + f.suffix + ".forke.test"
	if _, err := f.admins.Exec(context.Background(), `INSERT INTO admins (email, name, username, password_hash, role) VALUES ($1, $2, $3, $4, $5)`,
		email, name, name+f.suffix, string(hash), role); err != nil {
		f.t.Fatal(err)
	}
	return email
}

func (f *forkeFixture) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

func (f *forkeFixture) login(login, password string) (*http.Cookie, int) {
	w := f.do("POST", "/api/auth/login", `{"email":"`+login+`","password":"`+password+`"}`, nil)
	if w.Code != 200 {
		return nil, w.Code
	}
	return w.Result().Cookies()[0], w.Code
}

func TestForkeSuperAdminLogin(t *testing.T) {
	f := newForkeFixture(t)
	email := f.addAdmin("root", forke.RoleSuperAdmin, "correct horse")
	cookie, code := f.login(strings.ToUpper(email), "correct horse")
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	if w := f.do("GET", "/api/me", "", cookie); w.Code != 200 || !strings.Contains(w.Body.String(), `"role":"super_admin"`) {
		t.Fatalf("me %d %s", w.Code, w.Body)
	}
	if w := f.do("POST", "/api/keys", `{"name":"forke-admin","scope":"full_access","password":"correct horse"}`, cookie); w.Code != 201 {
		t.Fatalf("create key %d %s", w.Code, w.Body)
	}
	if w := f.do("POST", "/api/keys", `{"name":"x","scope":"full_access","password":"wrong"}`, cookie); w.Code != 403 {
		t.Fatalf("wrong password key %d", w.Code)
	}
	if _, code := f.login("root"+f.suffix, "correct horse"); code != 200 {
		t.Fatalf("username login %d", code)
	}
}

func TestForkeAdminRoleIsLimited(t *testing.T) {
	f := newForkeFixture(t)
	email := f.addAdmin("editor", forke.RoleAdmin, "editor pass")
	cookie, code := f.login(email, "editor pass")
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	for route, want := range map[string]int{"GET /api/contacts": 200, "GET /api/keys": 403, "GET /api/domains": 200} {
		method, path, _ := strings.Cut(route, " ")
		if w := f.do(method, path, "", cookie); w.Code != want {
			t.Fatalf("%s: %d", route, w.Code)
		}
	}
	if w := f.do("POST", "/api/domains", `{"name":"x.forke.test"}`, cookie); w.Code != 403 {
		t.Fatalf("admin created domain: %d", w.Code)
	}
}

func TestForkeRoleChangesApplyImmediately(t *testing.T) {
	f := newForkeFixture(t)
	email := f.addAdmin("lead", forke.RoleSuperAdmin, "lead pass")
	cookie, _ := f.login(email, "lead pass")
	if w := f.do("GET", "/api/keys", "", cookie); w.Code != 200 {
		t.Fatalf("keys before demotion %d", w.Code)
	}
	if _, err := f.admins.Exec(context.Background(), `UPDATE admins SET role = 'admin' WHERE email = $1`, email); err != nil {
		t.Fatal(err)
	}
	if w := f.do("GET", "/api/keys", "", cookie); w.Code != 403 {
		t.Fatalf("keys after demotion %d", w.Code)
	}
	if _, err := f.admins.Exec(context.Background(), `UPDATE admins SET is_disabled = true WHERE email = $1`, email); err != nil {
		t.Fatal(err)
	}
	if w := f.do("GET", "/api/me", "", cookie); w.Code != 401 {
		t.Fatalf("disabled admin still signed in: %d", w.Code)
	}
}

func TestForkeLoginRejections(t *testing.T) {
	f := newForkeFixture(t)
	email := f.addAdmin("someone", forke.RoleAdmin, "right pass")
	invited := f.addAdmin("invited", forke.RoleAdmin, "x")
	if _, err := f.admins.Exec(context.Background(), `UPDATE admins SET password_hash = NULL WHERE email = $1`, invited); err != nil {
		t.Fatal(err)
	}
	other := f.addAdmin("viewer", "viewer", "viewer pass")
	for _, c := range [][2]string{{email, "wrong pass"}, {"nobody@" + f.suffix + ".forke.test", "right pass"}, {invited, "x"}, {other, "viewer pass"}} {
		if _, code := f.login(c[0], c[1]); code != 401 {
			t.Fatalf("%s accepted: %d", c[0], code)
		}
	}
	if w := f.do("POST", "/api/auth/signup", `{}`, nil); w.Code == 201 || w.Code == 200 {
		t.Fatalf("signup still open: %d", w.Code)
	}
}
