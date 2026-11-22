// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"strings"
	"testing"
)

func TestPublicStatus(t *testing.T) {
	f := newAPIFixture(t)
	w := f.request("GET", "/api/public/status", "", "")
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	body := decodeJSON(t, w.Body.String())
	windows, ok := body["windows"].(map[string]any)
	if !ok || windows["24h"] == nil || windows["7d"] == nil || windows["30d"] == nil {
		t.Fatalf("windows missing: %s", w.Body)
	}
	components, _ := body["components"].([]any)
	if len(components) != 6 {
		t.Fatalf("components: %s", w.Body)
	}
	if body["infrastructure"] != "Amazon SES" || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("headers or body: %v %s", w.Header(), w.Body)
	}
	if strings.Contains(w.Body.String(), f.ws) {
		t.Fatal("public status leaked a workspace id")
	}
	for _, path := range []string{"/status", "/status/"} {
		r := f.request("GET", path, "", "")
		if r.Code != 302 || !strings.HasSuffix(r.Header().Get("Location"), "#mail") {
			t.Fatalf("%s redirect %d %q", path, r.Code, r.Header().Get("Location"))
		}
	}
}
