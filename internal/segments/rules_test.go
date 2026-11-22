// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package segments

import (
	"strings"
	"testing"
)

func TestCompileGroups(t *testing.T) {
	sql, args, err := Compile("all", []Rule{
		{Field: "tag", Op: "has", Value: "Subscribers"},
		{Field: "group", Op: "any", Rules: []Rule{{Field: "last_engaged_at", Op: "within_days", Days: 90}, {Field: "created_at", Op: "within_days", Days: 90}}},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, " OR ") || !strings.Contains(sql, "$3") || !strings.Contains(sql, "$5") || len(args) != 3 {
		t.Fatalf("%s %v", sql, args)
	}
	if _, _, err := Compile("all", []Rule{{Field: "group", Op: "any", Rules: []Rule{{Field: "group", Op: "any", Rules: []Rule{{Field: "email", Op: "is_set"}}}}}}, 0); err == nil {
		t.Fatal("nested group accepted")
	}
	if _, _, err := Compile("all", []Rule{{Field: "group", Op: "any"}}, 0); err == nil {
		t.Fatal("empty group accepted")
	}
}
