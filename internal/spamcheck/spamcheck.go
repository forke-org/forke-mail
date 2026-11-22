package spamcheck

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/quotedprintable"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Rule struct {
	Name        string  `json:"name"`
	Score       float64 `json:"score"`
	Description string  `json:"description"`
}

type Result struct {
	Available bool     `json:"available"`
	Score     float64  `json:"score"`
	Threshold float64  `json:"threshold"`
	Spam      bool     `json:"spam"`
	Rules     []Rule   `json:"rules"`
	Warnings  []string `json:"warnings"`
}

type Checker struct {
	Addr      string
	Threshold float64
}

type Email struct {
	FromName, FromAddress, Subject, HTML, Text string
}

var shorteners = map[string]bool{
	"bit.ly": true, "tinyurl.com": true, "goo.gl": true, "t.co": true, "ow.ly": true, "is.gd": true, "cutt.ly": true,
	"rebrand.ly": true, "buff.ly": true, "shorturl.at": true, "rb.gy": true, "tiny.cc": true, "bl.ink": true, "s.id": true,
}

var href = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)

func warnings(e Email) []string {
	out := []string{}
	for _, m := range href.FindAllStringSubmatch(e.HTML, -1) {
		u, err := url.Parse(strings.TrimSpace(m[1]))
		if err == nil && shorteners[strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))] {
			out = append(out, "Uses a link shortener ("+u.Hostname()+"). Spam filters distrust them; link to your own domain instead.")
			break
		}
	}
	letters, upper := 0, 0
	for _, r := range e.Subject {
		if r >= 'A' && r <= 'Z' {
			upper++
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			letters++
		}
	}
	if letters >= 8 && upper*10 >= letters*7 {
		out = append(out, "The subject is mostly capital letters.")
	}
	if strings.Count(e.Subject, "!") >= 2 {
		out = append(out, "The subject has several exclamation marks.")
	}
	if strings.TrimSpace(e.Text) == "" && strings.Count(strings.ToLower(e.HTML), "<img") > 0 && len(stripTags(e.HTML)) < 200 {
		out = append(out, "The email is mostly images with little text.")
	}
	return out
}

var tags = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string {
	return strings.TrimSpace(tags.ReplaceAllString(s, " "))
}

func message(e Email) []byte {
	var b bytes.Buffer
	boundary := "forke-mail-spamcheck"
	fmt.Fprintf(&b, "From: %q <%s>\r\nTo: <check@example.com>\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <spamcheck-%d@forke.space>\r\nMIME-Version: 1.0\r\n",
		e.FromName, e.FromAddress, strings.ReplaceAll(e.Subject, "\n", " "), time.Now().UTC().Format(time.RFC1123Z), time.Now().UnixNano())
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	text := e.Text
	if strings.TrimSpace(text) == "" {
		text = stripTags(e.HTML)
	}
	for _, part := range [][2]string{{"text/plain", text}, {"text/html", e.HTML}} {
		if part[1] == "" {
			continue
		}
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, part[0])
		w := quotedprintable.NewWriter(&b)
		w.Write([]byte(part[1]))
		w.Close()
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

var header = regexp.MustCompile(`^Spam: (True|False|Yes|No) ; (-?[0-9.]+) / (-?[0-9.]+)`)
var ruleLine = regexp.MustCompile(`^\s*(-?[0-9.]+)\s+([A-Z0-9_]+)\s+(.*)$`)

func parse(raw string) (Result, error) {
	head, body, _ := strings.Cut(raw, "\r\n\r\n")
	res := Result{Available: true, Rules: []Rule{}}
	found := false
	for _, line := range strings.Split(head, "\r\n") {
		if m := header.FindStringSubmatch(line); m != nil {
			res.Spam = m[1] == "True" || m[1] == "Yes"
			res.Score, _ = strconv.ParseFloat(m[2], 64)
			res.Threshold, _ = strconv.ParseFloat(m[3], 64)
			found = true
		}
	}
	if !found {
		return res, errors.New("spamd: no score in reply")
	}
	inRules := false
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "----") {
			inRules = true
			continue
		}
		if !inRules || strings.TrimSpace(line) == "" {
			continue
		}
		if m := ruleLine.FindStringSubmatch(line); m != nil {
			score, _ := strconv.ParseFloat(m[1], 64)
			res.Rules = append(res.Rules, Rule{Name: m[2], Score: score, Description: strings.TrimSpace(m[3])})
		} else if n := len(res.Rules); n > 0 {
			res.Rules[n-1].Description += " " + strings.TrimSpace(line)
		}
	}
	return res, nil
}

func (c Checker) Check(ctx context.Context, e Email) Result {
	res := Result{Threshold: c.Threshold, Rules: []Rule{}}
	if c.Addr != "" {
		if r, err := c.ask(ctx, message(e)); err == nil {
			res = r
			res.Threshold = c.Threshold
			res.Spam = res.Score >= c.Threshold
		}
	}
	res.Warnings = warnings(e)
	return res
}

func (c Checker) ask(ctx context.Context, msg []byte) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	fmt.Fprintf(conn, "REPORT SPAMC/1.5\r\nContent-length: %d\r\nUser: forkemail\r\n\r\n", len(msg))
	if _, err := conn.Write(msg); err != nil {
		return Result{}, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.CloseWrite()
	}
	var out bytes.Buffer
	if _, err := out.ReadFrom(conn); err != nil {
		return Result{}, err
	}
	return parse(out.String())
}
