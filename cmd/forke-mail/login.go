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
		if r.URL.Query().Get("state") != state || r.URL.Query().Get("code") == "" {
			fmt.Fprintln(w, "Sign-in was cancelled. You can close this tab.")
			codes <- ""
			return
		}
		fmt.Fprintln(w, "Signed in to Forke Mail. You can close this tab and go back to your terminal.")
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
