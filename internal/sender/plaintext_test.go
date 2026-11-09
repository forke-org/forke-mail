package sender

import "testing"

func TestPlainTextFromHTML(t *testing.T) {
	got := PlainText(`<html><head><style>p{color:red}</style></head><body><h1>Welcome&nbsp;in</h1><p>You are   on the list.</p><p><a href="https://forke.space/x?a=1&amp;b=2">Read more</a><br>Bye &amp; thanks</p></body></html>`)
	want := "Welcome in\nYou are on the list.\nRead more (https://forke.space/x?a=1&b=2)\nBye & thanks\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
}
