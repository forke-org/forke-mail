// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package templates

import (
	"strings"
	"testing"
)

func TestFillRawAndEscaped(t *testing.T) {
	got := fill(`<p>{{ name }}</p>{{{block}}}{{{missing}}}{{unknown}}`, map[string]string{"name": "<b>", "block": "<i>x</i>"}, true)
	if got != `<p>&lt;b&gt;</p><i>x</i>{{unknown}}` {
		t.Fatalf("%q", got)
	}
	if vars := strings.Join(Variables("{{a}} {{{b}}}", "{{a}}"), ","); vars != "a,b" {
		t.Fatalf("vars %s", vars)
	}
}
