package sender

import (
	"strings"
	"testing"
)

func TestRenderEscapesMergeFieldsInHTML(t *testing.T) {
	m := Render(Content{Subject: "Hi {{first_name}}", FromName: "Forke", FromAddress: "a@forke.space",
		HTML: "<html><body><p>{{first_name}} {{email}}</p></body></html>", Text: "Hi {{first_name}}"},
		Recipient{Email: "x@y.io", FirstName: "<script>"}, "https://mail.forke.space/unsubscribe/tok")
	if strings.Contains(m.HTML, "<script>") || !strings.Contains(m.HTML, "&lt;script&gt;") {
		t.Fatalf("html not escaped: %s", m.HTML)
	}
	if !strings.Contains(m.HTML, `href="https://mail.forke.space/unsubscribe/tok">Unsubscribe</a></p></body>`) {
		t.Fatalf("footer not before </body>: %s", m.HTML)
	}
	if m.Text != "Hi <script>\n\nUnsubscribe: https://mail.forke.space/unsubscribe/tok\n" {
		t.Fatalf("text = %q", m.Text)
	}
	if m.Headers["List-Unsubscribe"] != "<https://mail.forke.space/unsubscribe/tok>" || m.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Fatalf("headers = %v", m.Headers)
	}
}
