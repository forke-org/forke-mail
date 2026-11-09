package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/domains"
	"forkemail/internal/events"
	"forkemail/internal/ratelimit"
	"forkemail/internal/webhooks"
)

type publicDNS struct{}

func (publicDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
}

func (f *apiFixture) developer(d *developerServices) {
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	f.router = newRouter(testConfig("development"), f.pool, lim, services{developer: d, identities: domains.Local{}, sendRate: func() float64 { return 20 }})
}

func TestCustomerWebhookManagement(t *testing.T) {
	f := newAPIFixture(t)
	v, err := webhooks.NewVault(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool, Vault: v, Network: webhooks.Network{Resolver: publicDNS{}}}})
	w := f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com/events","events":["email.delivered"]}`, "")
	if w.Code != 201 {
		t.Fatalf("create %d", w.Code)
	}
	var out struct {
		ID, Secret    string
		SecretVersion int `json:"secret_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Secret == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("one-time secret not protected")
	}
	w = f.request("GET", "/api/webhooks", "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), out.Secret) || strings.Contains(w.Body.String(), `"secret":`) {
		t.Fatal("list leaked signing secret")
	}
	stored := dbtest.QueryString(t, f.pool, `SELECT secret FROM webhook_endpoints WHERE id=$1 AND workspace_id=$2`, out.ID, f.ws)
	if stored == out.Secret || !strings.HasPrefix(stored, "enc1:") {
		t.Fatal("plaintext secret stored")
	}
	other := newAPIFixture(t)
	other.developer(&developerServices{hooks: &webhooks.Handler{Pool: other.pool, Vault: v, Network: webhooks.Network{Resolver: publicDNS{}}}})
	for _, tc := range []struct{ method, path, body string }{{"PATCH", "/api/webhooks/" + out.ID, `{"enabled":false}`}, {"DELETE", "/api/webhooks/" + out.ID, ""}, {"POST", "/api/webhooks/" + out.ID + "/rotate", ""}, {"GET", "/api/webhooks/" + out.ID + "/deliveries", ""}} {
		if code := other.request(tc.method, tc.path, tc.body, "").Code; code != 404 {
			t.Fatalf("foreign webhook %s %d", tc.path, code)
		}
	}
	if err := events.Record(context.Background(), f.pool, events.Event{WorkspaceID: f.ws, Type: "delivered"}); err != nil {
		t.Fatal(err)
	}
	if code := f.request("GET", "/api/webhooks/"+out.ID+"/deliveries", "", "").Code; code != 200 {
		t.Fatalf("deliveries %d", code)
	}
	w = f.request("POST", "/api/webhooks/"+out.ID+"/rotate", "", "")
	var rotated struct {
		Secret  string
		Version int `json:"secret_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 || rotated.Secret == out.Secret || rotated.Version != 2 {
		t.Fatal("rotation failed")
	}
	if code := f.request("PATCH", "/api/webhooks/"+out.ID, `{"enabled":false}`, "").Code; code != 200 {
		t.Fatalf("disable %d", code)
	}
	if s := dbtest.QueryString(t, f.pool, `SELECT status FROM webhook_deliveries WHERE endpoint_id=$1 AND workspace_id=$2`, out.ID, f.ws); s != "cancelled" {
		t.Fatal("disable left pending jobs")
	}
	if code := f.request("DELETE", "/api/webhooks/"+out.ID, "", "").Code; code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	if code := f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com","events":["email.unsupported"]}`, "").Code; code != 422 {
		t.Fatal("unsupported event accepted")
	}
}

func TestDomainDNSRouteIsolation(t *testing.T) {
	f := newAPIFixture(t)
	id := dbtest.QueryString(t, f.pool, `INSERT INTO domains(workspace_id,name,kind) VALUES($1,$2,'domain') RETURNING id`, f.ws, f.ws+".example.com")
	other := newAPIFixture(t)
	if code := other.request("GET", "/api/domains/"+id+"/dns", "", "").Code; code != 404 {
		t.Fatalf("foreign domain %d", code)
	}
	if code := f.request("GET", "/api/domains/"+id+"/dns?mail_from=attacker.example.com", "", "").Code; code != 422 {
		t.Fatalf("foreign MAIL FROM %d", code)
	}
}
