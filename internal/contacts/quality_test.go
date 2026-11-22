// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package contacts

import "testing"

func TestAssess(t *testing.T) {
	cases := map[string]Quality{
		"ada@gmail.com":        {},
		"info@acme.com":        {Reason: "role"},
		"support2@acme.com":    {Reason: "role"},
		"x@mailinator.com":     {Reason: "disposable"},
		"bob@gmial.com":        {Reason: "typo", Suggestion: "bob@gmail.com"},
		"information@acme.com": {},
		"hello.world@acme.com": {},
	}
	for email, want := range cases {
		if got := Assess(email); got != want {
			t.Errorf("%s = %+v, want %+v", email, got, want)
		}
	}
}

func TestCleanAndCheck(t *testing.T) {
	p := Parsed{Rows: []Row{{Email: "ada@gmail.com"}, {Email: "info@acme.com"}, {Email: "x@yopmail.com"}, {Email: "b@hotmial.com"}}}
	rep := p.Check()
	if rep.Role != 1 || rep.Disposable != 1 || rep.Typos != 1 || len(rep.Samples) != 3 {
		t.Fatalf("report %+v", rep)
	}
	if out, dropped := p.Clean(false); len(out.Rows) != 1 || dropped != 3 {
		t.Fatalf("strict %d %d", len(out.Rows), dropped)
	}
	if out, dropped := p.Clean(true); len(out.Rows) != 3 || dropped != 1 {
		t.Fatalf("keep risky %d %d", len(out.Rows), dropped)
	}
}
