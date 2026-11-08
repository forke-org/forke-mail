package contacts

import (
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := "Email,First_Name,last_name\n" +
		" Ada@Example.com ,Ada,Lovelace\n" +
		"ada@example.com,Dup,\n" +
		"not-an-email,X,\n" +
		"@missing.com,,\n" +
		"bob@example,,\n" +
		"bob@example.com,\"Bo\nb\",\n"
	p, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 2 || p.Invalid != 3 || p.Duplicates != 1 {
		t.Fatalf("rows=%d invalid=%d dupes=%d", len(p.Rows), p.Invalid, p.Duplicates)
	}
	if p.Rows[0] != (Row{Email: "ada@example.com", FirstName: "Ada", LastName: "Lovelace"}) {
		t.Fatalf("unexpected first row %+v", p.Rows[0])
	}
	if p.Rows[1].FirstName != "Bob" {
		t.Fatalf("control characters not stripped: %q", p.Rows[1].FirstName)
	}
}

func TestParseCSVWithoutHeader(t *testing.T) {
	p, err := ParseCSV(strings.NewReader("a@x.io\nb@x.io,Bee\n"))
	if err != nil || len(p.Rows) != 2 || p.Rows[1].FirstName != "Bee" {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestParseCSVRowCap(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= maxImportRows; i++ {
		b.WriteString("x@y.io\n")
	}
	if _, err := ParseCSV(strings.NewReader(b.String())); err != errTooManyRows {
		t.Fatalf("got %v, want errTooManyRows", err)
	}
}
