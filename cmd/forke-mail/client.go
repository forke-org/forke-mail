// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type client struct {
	base   string
	apiKey string
	creds  credentials
	http   *http.Client
}

func newClient(base string) (*client, error) {
	c := &client{base: base, http: &http.Client{Timeout: 60 * time.Second}}
	if key := os.Getenv("FORKE_MAIL_API_KEY"); key != "" {
		c.apiKey = key
		return c, nil
	}
	creds, err := loadCredentials()
	if err != nil {
		return nil, err
	}
	c.creds = creds
	if creds.BaseURL != "" {
		c.base = creds.BaseURL
	}
	return c, nil
}

func (c *client) token() (string, error) {
	if c.apiKey != "" {
		return c.apiKey, nil
	}
	if time.Until(c.creds.ExpiresAt) < time.Minute {
		if err := c.creds.refresh(); err != nil {
			return "", err
		}
	}
	return c.creds.AccessToken, nil
}

func (c *client) post(body []byte) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := c.token()
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest("POST", c.base+"/mcp", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		out, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized && c.apiKey == "" && attempt == 0 {
			if err := c.creds.refresh(); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("Forke Mail rejected the credentials: run `forke-mail login` or check FORKE_MAIL_API_KEY")
		}
		if res.StatusCode == http.StatusAccepted {
			return nil, nil
		}
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("Forke Mail returned %d: %s", res.StatusCode, strings.TrimSpace(string(out)))
		}
		return out, nil
	}
	return nil, errors.New("could not authenticate")
}

func (c *client) rpc(method string, params any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	out, err := c.post(body)
	if err != nil {
		return nil, err
	}
	var res struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, errors.New(res.Error.Message)
	}
	return res.Result, nil
}

type toolResult struct {
	Text    string
	IsError bool
}

func (c *client) tool(name string, args map[string]any) (toolResult, error) {
	raw, err := c.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return toolResult{}, err
	}
	var res struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return toolResult{}, err
	}
	var parts []string
	for _, c := range res.Content {
		parts = append(parts, c.Text)
	}
	return toolResult{Text: strings.Join(parts, "\n"), IsError: res.IsError}, nil
}

var (
	confirmToken = regexp.MustCompile(`"confirm_token": "([^"]+)"`)
	approvalID   = regexp.MustCompile(`"approval_id": "([^"]+)"`)
	approvalLink = regexp.MustCompile(`https?://\S+/approve/[0-9a-f-]+`)
)

func ask(question string) bool {
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	fmt.Fprint(os.Stderr, question+" [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func (c *client) run(name string, args map[string]any, yes bool) (toolResult, error) {
	res, err := c.tool(name, args)
	if err != nil {
		return res, err
	}
	if strings.HasPrefix(res.Text, "CONFIRMATION NEEDED") {
		m := confirmToken.FindStringSubmatch(res.Text)
		summary := strings.TrimPrefix(strings.SplitN(res.Text, "\n", 2)[0], "CONFIRMATION NEEDED: ")
		if m == nil {
			return res, nil
		}
		if !yes && !ask(summary+"\nGo ahead?") {
			return toolResult{Text: "Cancelled.", IsError: true}, nil
		}
		args["confirm_token"] = m[1]
		return c.tool(name, args)
	}
	if strings.HasPrefix(res.Text, "APPROVAL NEEDED") {
		m := approvalID.FindStringSubmatch(res.Text)
		link := approvalLink.FindString(res.Text)
		if m == nil || link == "" {
			return res, nil
		}
		fmt.Fprintln(os.Stderr, strings.SplitN(res.Text, "\n", 2)[0])
		fmt.Fprintln(os.Stderr, "Approve it in your browser: "+link)
		openBrowser(link)
		args["approval_id"] = m[1]
		deadline := time.Now().Add(30 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(3 * time.Second)
			res, err = c.tool(name, args)
			if err != nil || !strings.Contains(res.Text, "Still waiting") {
				return res, err
			}
		}
		return toolResult{Text: "The approval request expired.", IsError: true}, nil
	}
	return res, nil
}

func (c *client) bridge(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 4<<20)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		res, err := c.post(line)
		if err != nil {
			var req struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(line, &req) == nil && len(req.ID) > 0 {
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32000, "message": err.Error()}})
			}
			continue
		}
		if len(bytes.TrimSpace(res)) > 0 {
			if _, err := out.Write(append(bytes.TrimSpace(res), '\n')); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
