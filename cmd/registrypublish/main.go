// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const registry = "https://registry.modelcontextprotocol.io"

func main() {
	keyPath := flag.String("key", os.Getenv("HOME")+"/.config/forke/mcp-registry.key", "ed25519 seed (hex), created by -keygen")
	keygen := flag.Bool("keygen", false, "create a signing key and print the DNS TXT record")
	domain := flag.String("domain", "forke.space", "domain that owns the server name")
	file := flag.String("server", "server.json", "server.json to publish")
	flag.Parse()
	if err := run(*keyPath, *keygen, *domain, *file); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(keyPath string, keygen bool, domain, file string) error {
	if keygen {
		if _, err := os.Stat(keyPath); err == nil {
			return fmt.Errorf("%s already exists", keyPath)
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(keyPath[:strings.LastIndex(keyPath, "/")], 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Printf("Add this TXT record at %s:\nv=MCPv1; k=ed25519; p=%s\n", domain, base64.StdEncoding.EncodeToString(pub))
		return nil
	}
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return fmt.Errorf("%s is not an ed25519 seed", keyPath)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), []byte(stamp))
	var auth struct {
		Token string `json:"registry_token"`
	}
	if err := call("POST", "/v0/auth/dns", "", map[string]string{"domain": domain, "timestamp": stamp, "signed_timestamp": hex.EncodeToString(sig)}, &auth); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var server any
	if err := json.Unmarshal(body, &server); err != nil {
		return err
	}
	var out map[string]any
	if err := call("POST", "/v0/publish", auth.Token, server, &out); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	fmt.Println("Published", file)
	return nil
}

func call(method, path, token string, body, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(method, registry+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("%d %s", res.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}
