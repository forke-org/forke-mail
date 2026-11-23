// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type credentials struct {
	BaseURL      string    `json:"base_url"`
	ClientID     string    `json:"client_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func credentialsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "forke-mail", "credentials.json")
}

func loadCredentials() (credentials, error) {
	var c credentials
	b, err := os.ReadFile(credentialsPath())
	if err != nil {
		return c, errNotLoggedIn
	}
	return c, json.Unmarshal(b, &c)
}

func (c credentials) save() error {
	p := credentialsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

var errNotLoggedIn = errors.New("not signed in: run `forke-mail login`, or set FORKE_MAIL_API_KEY")

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var noBrowser bool

func openBrowser(link string) {
	if noBrowser {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", link)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		cmd = exec.Command("xdg-open", link)
	}
	_ = cmd.Start()
}

func postForm(endpoint string, form url.Values) (map[string]any, error) {
	res, err := http.PostForm(endpoint, form)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %v", endpoint, out["error"])
	}
	return out, nil
}

func login(base string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	reg, _ := json.Marshal(map[string]any{"client_name": "Forke Mail CLI", "redirect_uris": []string{"http://127.0.0.1/callback"}})
	res, err := http.Post(base+"/oauth/register", "application/json", strings.NewReader(string(reg)))
	if err != nil {
		return err
	}
	var client struct {
		ClientID string `json:"client_id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&client)
	res.Body.Close()
	if client.ClientID == "" {
		return errors.New("could not register the CLI with Forke Mail")
	}
	verifier := randomString(32)
	sum := sha256.Sum256([]byte(verifier))
	state := randomString(16)
	link := base + "/oauth/authorize?" + url.Values{
		"response_type": {"code"}, "client_id": {client.ClientID}, "redirect_uri": {redirect},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "state": {state},
	}.Encode()
	codes := make(chan string, 1)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Query().Get("state") != state || r.URL.Query().Get("code") == "" {
			fmt.Fprint(w, resultPage(false))
			codes <- ""
			return
		}
		fmt.Fprint(w, resultPage(true))
		codes <- r.URL.Query().Get("code")
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())
	fmt.Fprintln(os.Stderr, "Opening your browser to sign in. If it does not open, visit:\n  "+link)
	openBrowser(link)
	var code string
	select {
	case code = <-codes:
	case <-time.After(10 * time.Minute):
		return errors.New("timed out waiting for sign-in")
	}
	if code == "" {
		return errors.New("sign-in was cancelled")
	}
	tok, err := postForm(base+"/oauth/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {client.ClientID}, "redirect_uri": {redirect}, "code_verifier": {verifier}})
	if err != nil {
		return err
	}
	return credentialsFrom(base, client.ClientID, tok).save()
}

func credentialsFrom(base, clientID string, tok map[string]any) credentials {
	access, _ := tok["access_token"].(string)
	refresh, _ := tok["refresh_token"].(string)
	secs, _ := tok["expires_in"].(float64)
	return credentials{BaseURL: base, ClientID: clientID, AccessToken: access, RefreshToken: refresh, ExpiresAt: time.Now().Add(time.Duration(secs) * time.Second)}
}

func (c *credentials) refresh() error {
	tok, err := postForm(c.BaseURL+"/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}, "client_id": {c.ClientID}})
	if err != nil {
		return fmt.Errorf("your sign-in expired: run `forke-mail login` (%w)", err)
	}
	*c = credentialsFrom(c.BaseURL, c.ClientID, tok)
	return c.save()
}

func logout() error {
	c, err := loadCredentials()
	if err == nil {
		_, _ = postForm(c.BaseURL+"/oauth/revoke", url.Values{"token": {c.RefreshToken}})
	}
	if err := os.Remove(credentialsPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintln(os.Stderr, "Signed out.")
	return nil
}

func resultPage(ok bool) string {
	title, body, icon, tone := "You're signed in", "The Forke Mail CLI is connected. You can close this tab and go back to your terminal.", "&#10003;", "#4ade80"
	if !ok {
		title, body, icon, tone = "Sign-in cancelled", "Nothing was connected. Run forke-mail login again whenever you're ready.", "&#10005;", "#f87171"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>` + title + ` · Forke Mail</title>
<style>
*{box-sizing:border-box}html,body{margin:0;height:100%}
body{display:grid;place-items:center;padding:24px;background:radial-gradient(900px 520px at 50% 115%,rgba(255,122,0,.28),transparent 60%),radial-gradient(rgba(240,227,222,.07) 1px,transparent 1px) 0 0/14px 14px,#0b0604;color:#f4f4f5;font:15px/1.6 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;-webkit-font-smoothing:antialiased}
main{max-width:420px;text-align:center;animation:rise .6s cubic-bezier(.2,.7,.2,1) both}
.mark{font-size:22px;letter-spacing:-.04em;font-weight:600;margin:0 0 36px}.mark span{color:#ff7a00}.mark em{font-style:normal;font-weight:400;color:rgba(244,244,245,.6);margin-left:6px}
.icon{display:inline-grid;place-items:center;width:56px;height:56px;border-radius:50%;font-size:24px;color:` + tone + `;background:color-mix(in srgb,` + tone + ` 12%,transparent);box-shadow:0 0 0 1px color-mix(in srgb,` + tone + ` 30%,transparent),0 0 40px color-mix(in srgb,` + tone + ` 25%,transparent);margin-bottom:20px;animation:pop .5s .15s cubic-bezier(.2,.9,.3,1.3) both}
h1{margin:0 0 8px;font-size:24px;font-weight:600;letter-spacing:-.02em}p{margin:0;color:rgba(244,244,245,.62)}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;padding:2px 6px;border-radius:6px;background:rgba(255,255,255,.06)}
@keyframes rise{from{opacity:0;transform:translateY(10px)}to{opacity:1;transform:none}}@keyframes pop{from{opacity:0;transform:scale(.6)}to{opacity:1;transform:none}}
</style></head><body><main><p class="mark">forke<span>*</span><em>mail</em></p><div class="icon">` + icon + `</div><h1>` + title + `</h1><p>` + body + `</p></main></body></html>`
}
