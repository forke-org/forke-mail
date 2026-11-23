// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const version = "1.0.2"

const usage = `Forke Mail CLI

Usage:
  forke-mail login                 Sign in with your browser
  forke-mail logout                Sign out and revoke this device
  forke-mail whoami                Show the project, credits and key mode
  forke-mail init                  Set up email in this project: test key in .env and AGENTS.md notes
  forke-mail send --to a@b.com --from "Acme <hi@acme.com>" --subject Hi --text "Hello"
  forke-mail domains               List sending domains
  forke-mail tools                 List everything the CLI and AI agents can do
  forke-mail call <tool> ['{"json":"args"}' | key=value ...]
  forke-mail mcp                   Run as a local MCP server (stdio) for AI apps
  forke-mail version

Options: --url to use another server (or FORKE_MAIL_URL), --yes to skip confirmation prompts, --json for raw output,
--no-browser to print links instead of opening them.
Set FORKE_MAIL_API_KEY to use an API key instead of signing in (CI, servers).
Risky actions ask you to confirm in the terminal first; ones that can't be undone say so.
`

type options struct {
	base string
	yes  bool
	json bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts := options{base: os.Getenv("FORKE_MAIL_URL")}
	if opts.base == "" {
		opts.base = "https://mail.forke.space"
	}
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--yes" || a == "-y":
			opts.yes = true
		case a == "--no-browser":
			noBrowser = true
		case a == "--json":
			opts.json = true
		case a == "--url" && i+1 < len(args):
			opts.base = args[i+1]
			i++
		case strings.HasPrefix(a, "--url="):
			opts.base = strings.TrimPrefix(a, "--url=")
		default:
			rest = append(rest, a)
		}
	}
	opts.base = strings.TrimRight(opts.base, "/")
	if len(rest) == 0 {
		fmt.Print(usage)
		return nil
	}
	cmd, rest := rest[0], rest[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version", "--version":
		fmt.Println("forke-mail", version)
		return nil
	case "login":
		if err := login(opts.base); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Signed in.")
		return show(opts, "get_account", map[string]any{})
	case "logout":
		return logout()
	}
	c, err := newClient(opts.base)
	if err != nil {
		return err
	}
	switch cmd {
	case "mcp":
		return c.bridge(os.Stdin, os.Stdout)
	case "whoami":
		return c.show(opts, "get_account", map[string]any{})
	case "domains":
		return c.show(opts, "list_domains", map[string]any{})
	case "tools":
		return c.listTools(opts)
	case "send":
		return c.send(opts, rest)
	case "call":
		if len(rest) == 0 {
			return errors.New("usage: forke-mail call <tool> [json or key=value ...]")
		}
		args, err := parseArgs(rest[1:])
		if err != nil {
			return err
		}
		return c.show(opts, rest[0], args)
	case "init":
		return c.initProject(opts)
	}
	return fmt.Errorf("unknown command %q, run forke-mail help", cmd)
}

func show(opts options, tool string, args map[string]any) error {
	c, err := newClient(opts.base)
	if err != nil {
		return err
	}
	return c.show(opts, tool, args)
}

func (c *client) show(opts options, tool string, args map[string]any) error {
	res, err := c.run(tool, args, opts.yes)
	if err != nil {
		return err
	}
	fmt.Println(res.Text)
	if res.IsError {
		os.Exit(2)
	}
	return nil
}

func parseArgs(items []string) (map[string]any, error) {
	args := map[string]any{}
	if len(items) == 1 && strings.HasPrefix(strings.TrimSpace(items[0]), "{") {
		return args, json.Unmarshal([]byte(items[0]), &args)
	}
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value, got %q", it)
		}
		var parsed any
		if json.Unmarshal([]byte(v), &parsed) == nil {
			args[k] = parsed
		} else {
			args[k] = v
		}
	}
	return args, nil
}

func (c *client) listTools(opts options) error {
	raw, err := c.rpc("tools/list", map[string]any{})
	if err != nil {
		return err
	}
	if opts.json {
		fmt.Println(string(raw))
		return nil
	}
	var res struct {
		Tools []struct {
			Name, Description string
		}
	}
	_ = json.Unmarshal(raw, &res)
	for _, t := range res.Tools {
		fmt.Printf("%-18s %s\n", t.Name, t.Description)
	}
	return nil
}

func (c *client) send(opts options, argv []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	to := fs.String("to", "", "recipient")
	from := fs.String("from", "", `sender, "Name <address>"`)
	subject := fs.String("subject", "", "subject")
	text := fs.String("text", "", "plain-text body")
	html := fs.String("html", "", "HTML body")
	htmlFile := fs.String("html-file", "", "read the HTML body from a file")
	template := fs.String("template", "", "template name or ID")
	var data multi
	fs.Var(&data, "data", "template variable key=value, repeatable")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *htmlFile != "" {
		b, err := os.ReadFile(*htmlFile)
		if err != nil {
			return err
		}
		*html = string(b)
	}
	args := map[string]any{"to": *to, "from": *from, "subject": *subject, "text": *text, "html": *html, "template": *template}
	if len(data) > 0 {
		d := map[string]any{}
		for _, kv := range data {
			k, v, _ := strings.Cut(kv, "=")
			d[k] = v
		}
		args["data"] = d
	}
	return c.show(opts, "send_email", args)
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

var keyPattern = regexp.MustCompile(`fm_(test|live)_[0-9a-f]{64}`)

const agentNotes = `
## Email (Forke Mail)

- Send email with Forke Mail: POST https://mail.forke.space/api/transactional with header Authorization: Bearer $FORKE_MAIL_API_KEY. Full reference: https://mail.forke.space/llms.txt
- FORKE_MAIL_API_KEY in .env is a test key (fm_test_…): emails are recorded with status "test" and never delivered. Use a live key (fm_live_…) from the Forke Mail dashboard in production.
- Read the key from the environment, never hard-code or commit it. Send an Idempotency-Key header for retries.
- Keep repeated emails as templates (template + data) instead of HTML in code.
- AI agents can also use the Forke Mail MCP server: https://mail.forke.space/mcp
`

func (c *client) initProject(opts options) error {
	if err := c.show(opts, "get_account", map[string]any{}); err != nil {
		return err
	}
	env, _ := os.ReadFile(".env")
	if strings.Contains(string(env), "FORKE_MAIL_API_KEY=") {
		fmt.Fprintln(os.Stderr, ".env already has FORKE_MAIL_API_KEY, keeping it.")
	} else {
		dir, _ := os.Getwd()
		res, err := c.run("create_api_key", map[string]any{"name": filepath.Base(dir) + " (test)", "mode": "test"}, opts.yes)
		if err != nil {
			return err
		}
		key := keyPattern.FindString(res.Text)
		if key == "" {
			return errors.New("no key was created: " + res.Text)
		}
		f, err := os.OpenFile(".env", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		prefix := ""
		if len(env) > 0 && !strings.HasSuffix(string(env), "\n") {
			prefix = "\n"
		}
		_, err = fmt.Fprintf(f, "%sFORKE_MAIL_API_KEY=%s\n", prefix, key)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Added a test key to .env as FORKE_MAIL_API_KEY.")
		ignore, _ := os.ReadFile(".gitignore")
		if !regexp.MustCompile(`(?m)^\.env\b`).Match(ignore) && (opts.yes || ask(".env is not in .gitignore. Add it so the key is never committed?")) {
			g, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err == nil {
				fmt.Fprintln(g, ".env")
				g.Close()
			}
		}
	}
	notes, _ := os.ReadFile("AGENTS.md")
	if !strings.Contains(string(notes), "Forke Mail") {
		f, err := os.OpenFile("AGENTS.md", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = f.WriteString(agentNotes)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Added Forke Mail notes to AGENTS.md so your AI coding agent knows how to send email here.")
	}
	fmt.Fprintf(os.Stderr, "\nNext: verify a domain with `forke-mail call add_domain name=yourdomain.com`, and connect your AI app to %s/mcp (or run `forke-mail mcp`).\n", c.base)
	return nil
}
