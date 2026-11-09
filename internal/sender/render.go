package sender

import (
	"html"
	"mime"
	"net/mail"
	"strings"
)

type Content struct {
	Subject     string
	FromName    string
	FromAddress string
	ReplyTo     string
	HTML        string
	Text        string
}

type Recipient struct {
	ID          string
	EmailID     string
	WorkspaceID string
	ContactID   string
	Email       string
	FirstName   string
	Attempts    int
	ClaimToken  string
}

func Render(c Content, r Recipient, unsubscribeURL string) Message {
	from := (&mail.Address{Name: c.FromName, Address: c.FromAddress}).String()
	subject := merge(c.Subject, r, false)
	return Message{
		WorkspaceID: r.WorkspaceID,
		EmailID:     r.EmailID,
		RecipientID: r.ID,
		To:          r.Email,
		From:        from,
		ReplyTo:     c.ReplyTo,
		Subject:     subject,
		HTML:        withHTMLFooter(merge(c.HTML, r, true), unsubscribeURL),
		Text:        withTextFooter(merge(c.Text, r, false), unsubscribeURL),
		Headers: map[string]string{
			"From":                  from,
			"To":                    r.Email,
			"Subject":               mime.QEncoding.Encode("utf-8", subject),
			"MIME-Version":          "1.0",
			"List-Unsubscribe":      "<" + unsubscribeURL + ">",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		},
	}
}

func merge(template string, r Recipient, escape bool) string {
	firstName, email := r.FirstName, r.Email
	if escape {
		firstName, email = html.EscapeString(firstName), html.EscapeString(email)
	}
	return strings.NewReplacer("{{first_name}}", firstName, "{{email}}", email).Replace(template)
}

func withHTMLFooter(body, unsubscribeURL string) string {
	if hasUnsubscribeLink(body) {
		return fillUnsubscribeLink(body, html.EscapeString(unsubscribeURL))
	}
	footer := `<p style="margin-top:32px;font-size:12px;color:#6b7280"><a href="` +
		html.EscapeString(unsubscribeURL) + `">Manage email preferences or unsubscribe</a></p>`
	if i := strings.LastIndex(strings.ToLower(body), "</body>"); i >= 0 {
		return body[:i] + footer + body[i:]
	}
	return body + footer
}

var unsubscribePlaceholders = []string{"{{unsubscribe_url}}"}

func hasUnsubscribeLink(body string) bool {
	for _, p := range unsubscribePlaceholders {
		if strings.Contains(body, p) {
			return true
		}
	}
	return false
}

func fillUnsubscribeLink(body, url string) string {
	for _, p := range unsubscribePlaceholders {
		body = strings.ReplaceAll(body, p, url)
	}
	return body
}

func withTextFooter(body, unsubscribeURL string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	if hasUnsubscribeLink(body) {
		return fillUnsubscribeLink(body, unsubscribeURL)
	}
	return strings.TrimRight(body, "\n") + "\n\nManage preferences or unsubscribe: " + unsubscribeURL + "\n"
}
