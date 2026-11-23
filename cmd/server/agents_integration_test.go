// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
)

func (f apiFixture) raw(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

func (f apiFixture) oauthToken(t *testing.T) string {
	t.Helper()
	reg := f.raw(httptest.NewRequest("POST", "/oauth/register", strings.NewReader(`{"client_name":"Test agent","redirect_uris":["http://127.0.0.1/callback"]}`)))
	var client struct {
		ClientID string `json:"client_id"`
	}
	if reg.Code != 201 || json.Unmarshal(reg.Body.Bytes(), &client) != nil {
		t.Fatalf("register %d %s", reg.Code, reg.Body)
	}
	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	params := url.Values{"response_type": {"code"}, "client_id": {client.ClientID}, "redirect_uri": {"http://127.0.0.1:5555/callback"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "state": {"xyz"}}
	anon := f.raw(httptest.NewRequest("GET", "/oauth/authorize?"+params.Encode(), nil))
	if anon.Code != 302 || !strings.HasPrefix(anon.Header().Get("Location"), "/login/?next=") {
		t.Fatalf("anonymous authorize %d %s", anon.Code, anon.Header().Get("Location"))
	}
	page := httptest.NewRequest("GET", "/oauth/authorize?"+params.Encode(), nil)
	page.AddCookie(f.cookie)
	if w := f.raw(page); w.Code != 200 || !strings.Contains(w.Body.String(), "Connect Test agent") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action 'self' http://127.0.0.1:5555") {
		t.Fatalf("consent %d %v %s", w.Code, w.Header(), w.Body)
	}
	params.Set("workspace_id", f.ws)
	params.Set("decision", "allow")
	cross := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(params.Encode()))
	cross.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	cross.AddCookie(f.cookie)
	if w := f.raw(cross); w.Code != 403 {
		t.Fatalf("cross-site consent accepted %d", w.Code)
	}
	fresh := url.Values{}
	for k, v := range params {
		fresh[k] = v
	}
	fresh.Set("workspace_id", "new")
	fresh.Set("project_name", "Agent project "+f.ws[:8])
	create := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(fresh.Encode()))
	create.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	create.AddCookie(f.cookie)
	if w := f.raw(create); w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "code=") {
		t.Fatalf("new project consent %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, f.pool, `SELECT m.role FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id WHERE w.name = $1`, "Agent project "+f.ws[:8]); got != "super_admin" {
		t.Fatalf("new project membership %q", got)
	}
	dbtest.Exec(t, f.pool, `DELETE FROM workspaces WHERE name = $1`, "Agent project "+f.ws[:8])
	allow := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(params.Encode()))
	allow.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	allow.AddCookie(f.cookie)
	w := f.raw(allow)
	loc, _ := url.Parse(w.Header().Get("Location"))
	code := loc.Query().Get("code")
	if w.Code != 303 || code == "" || loc.Query().Get("state") != "xyz" || loc.Port() != "5555" {
		t.Fatalf("allow %d %s", w.Code, w.Header().Get("Location"))
	}
	exchange := func(form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return f.raw(r)
	}
	bad := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {client.ClientID}, "redirect_uri": {"http://127.0.0.1:5555/callback"}, "code_verifier": {"wrong" + verifier}}
	if w := exchange(bad); w.Code != 400 {
		t.Fatalf("wrong verifier accepted %d", w.Code)
	}
	allow = httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(params.Encode()))
	allow.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	allow.AddCookie(f.cookie)
	loc, _ = url.Parse(f.raw(allow).Header().Get("Location"))
	good := url.Values{"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")}, "client_id": {client.ClientID}, "redirect_uri": {"http://127.0.0.1:5555/callback"}, "code_verifier": {verifier}}
	w = exchange(good)
	var tok struct{ AccessToken, RefreshToken string }
	var raw map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &raw)
	tok.AccessToken, tok.RefreshToken = raw["access_token"], raw["refresh_token"]
	if w.Code != 200 || !strings.HasPrefix(tok.AccessToken, "fm_oauth_") {
		t.Fatalf("token %d %s", w.Code, w.Body)
	}
	if w := exchange(good); w.Code != 400 {
		t.Fatal("authorization code reused")
	}
	w = exchange(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok.RefreshToken}, "client_id": {client.ClientID}})
	_ = json.Unmarshal(w.Body.Bytes(), &raw)
	if w.Code != 200 || raw["access_token"] == tok.AccessToken {
		t.Fatalf("refresh %d %s", w.Code, w.Body)
	}
	return raw["access_token"]
}

func (f apiFixture) mcp(t *testing.T, token, method string, params any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := f.raw(r)
	if w.Code != 200 {
		t.Fatalf("%s %d %s", method, w.Code, w.Body)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["error"] != nil {
		t.Fatalf("%s error %v", method, out["error"])
	}
	return out["result"].(map[string]any)
}

func toolText(res map[string]any) string {
	return res["content"].([]any)[0].(map[string]any)["text"].(string)
}

var confirmPattern = regexp.MustCompile(`"confirm_token": "([^"]+)"`)

func TestAgentsOAuthAndMCP(t *testing.T) {
	f := newAPIFixture(t)
	unauth := f.raw(httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`)))
	if unauth.Code != 401 || !strings.Contains(unauth.Header().Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource") {
		t.Fatalf("unauthenticated mcp %d %v", unauth.Code, unauth.Header())
	}
	if w := f.raw(httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil)); !strings.Contains(w.Body.String(), "code_challenge_methods_supported") {
		t.Fatalf("metadata %s", w.Body)
	}
	for path, want := range map[string]string{"/llms.txt": "confirm_token", "/robots.txt": "Sitemap:", "/sitemap.xml": "/login/", "/.well-known/mcp.json": "streamable-http"} {
		if w := f.raw(httptest.NewRequest("GET", path, nil)); w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%s %d %s", path, w.Code, w.Body.String()[:min(300, w.Body.Len())])
		}
	}
	if w := f.raw(httptest.NewRequest("GET", "/docs/agents/", nil)); w.Code != 301 || w.Header().Get("Location") != "https://www.forke.space/docs/forke-mail" {
		t.Fatalf("docs redirect %d %s", w.Code, w.Header().Get("Location"))
	}
	token := f.oauthToken(t)

	init := f.mcp(t, token, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	if init["protocolVersion"] != "2025-06-18" {
		t.Fatalf("initialize %v", init)
	}
	list := f.mcp(t, token, "tools/list", map[string]any{})
	if b, _ := json.Marshal(list); !strings.Contains(string(b), `"send_broadcast"`) || !strings.Contains(string(b), `"destructiveHint":true`) {
		t.Fatalf("tools %s", b)
	}
	prompts := f.mcp(t, token, "prompts/list", map[string]any{})
	if list, _ := prompts["prompts"].([]any); len(list) < 5 {
		t.Fatalf("prompts %v", prompts)
	}
	got := f.mcp(t, token, "prompts/get", map[string]any{"name": "setup-app", "arguments": map[string]any{"framework": "Next.js"}})
	if b, _ := json.Marshal(got); !strings.Contains(string(b), "Set up Forke Mail in this Next.js project") {
		t.Fatalf("prompt %s", b)
	}
	if txt := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "get_account", "arguments": map[string]any{}})); !strings.Contains(txt, f.ws) {
		t.Fatalf("get_account %s", txt)
	}

	if w := f.request("POST", "/api/templates", `{"name":"agent-tpl","kind":"transactional","subject":"Hi","html":"<p>Hi</p>"}`, ""); w.Code != 201 {
		t.Fatalf("template %d %s", w.Code, w.Body)
	}
	if txt := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "delete_template", "arguments": map[string]any{"id": "agent-tpl"}})); !strings.HasPrefix(txt, "CONFIRMATION NEEDED") {
		t.Fatalf("delete_template by name: %s", txt)
	}
	if txt := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "get_template", "arguments": map[string]any{"id": "agent-tpl"}})); !strings.Contains(txt, `"name": "agent-tpl"`) {
		t.Fatalf("get_template by name: %s", txt)
	}

	domain := f.ws + ".agents.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	send := map[string]any{"to": "reader@example.com", "from": "Acme <hi@" + domain + ">", "subject": "Hello", "text": "Hi"}
	first := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "send_email", "arguments": send}))
	m := confirmPattern.FindStringSubmatch(first)
	if !strings.HasPrefix(first, "CONFIRMATION NEEDED") || m == nil {
		t.Fatalf("send without confirmation: %s", first)
	}
	if dbtest.QueryString(t, f.pool, `SELECT COUNT(*)::text FROM transactional_messages WHERE workspace_id = $1`, f.ws) != "0" {
		t.Fatal("sent before confirmation")
	}
	send["subject"] = "Changed"
	send["confirm_token"] = m[1]
	if txt := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "send_email", "arguments": send})); !strings.Contains(txt, "not valid") {
		t.Fatalf("token reused for different arguments: %s", txt)
	}
	send["subject"] = "Hello"
	res := f.mcp(t, token, "tools/call", map[string]any{"name": "send_email", "arguments": send})
	if res["isError"] == true || dbtest.QueryString(t, f.pool, `SELECT status FROM transactional_messages WHERE workspace_id = $1`, f.ws) != "queued" {
		t.Fatalf("confirmed send %v", res)
	}

	if txt := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "call_api", "arguments": map[string]any{"method": "POST", "path": "/api/account/password", "body": map[string]any{}}})); !strings.Contains(txt, "not available to AI agents") {
		t.Fatalf("blocked route ran: %s", txt)
	}
	roleArgs := map[string]any{"method": "PATCH", "path": "/api/team/members/" + f.ws, "body": map[string]any{"role": "admin"}}
	first = toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "call_api", "arguments": roleArgs}))
	serious := confirmPattern.FindStringSubmatch(first)
	if !strings.HasPrefix(first, "CONFIRMATION NEEDED (can't be undone)") || serious == nil {
		t.Fatalf("serious action not confirmed in chat: %s", first)
	}
	roleArgs["confirm_token"] = serious[1]
	if txt := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "call_api", "arguments": roleArgs})); strings.Contains(txt, "CONFIRMATION NEEDED") {
		t.Fatalf("confirmed serious action did not run: %s", txt)
	}
	if w := f.raw(httptest.NewRequest("GET", "/approve/"+f.ws, nil)); w.Code != 404 {
		t.Fatalf("browser approval page still served: %d", w.Code)
	}

	keyFirst := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "create_api_key", "arguments": map[string]any{"name": "dev"}}))
	km := confirmPattern.FindStringSubmatch(keyFirst)
	if km == nil {
		t.Fatalf("key creation not confirmed: %s", keyFirst)
	}
	keyRes := toolText(f.mcp(t, token, "tools/call", map[string]any{"name": "create_api_key", "arguments": map[string]any{"name": "dev", "confirm_token": km[1]}}))
	testKey := regexp.MustCompile(`fm_test_[0-9a-f]{64}`).FindString(keyRes)
	if testKey == "" {
		t.Fatalf("no test key: %s", keyRes)
	}
	testSend := map[string]any{"to": "someone@example.com", "from": "Anyone <x@unverified.example>", "subject": "Test", "text": "Hi"}
	if txt := toolText(f.mcp(t, testKey, "tools/call", map[string]any{"name": "send_email", "arguments": testSend})); strings.Contains(txt, "CONFIRMATION NEEDED") || !strings.Contains(txt, `"test"`) {
		t.Fatalf("test key send: %s", txt)
	}
	if w := f.request("POST", "/api/campaigns", `{"name":"x"}`, testKey); w.Code == 401 {
		t.Fatal("test key rejected by the REST API")
	}
}
