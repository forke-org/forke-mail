package auth

import "testing"

func TestScopeTable(t *testing.T) {
	cases := []struct {
		scope, route string
		want         bool
	}{
		{Session, "POST /api/keys", true},
		{FullAccess, "POST /api/keys", false},
		{SendingAccess, "GET /api/keys", false},
		{SendingAccess, "POST /api/contacts/import", true},
		{SendingAccess, "POST /api/emails/{id}/send", true},
		{SendingAccess, "GET /api/contacts", false},
		{SendingAccess, "DELETE /api/contacts/{id}", false},
		{FullAccess, "GET /api/contacts", true},
		{FullAccess, "GET /api/unknown", false},
		{SendingAccess, "GET /api/unknown", false},
		{FullAccess, "PUT /api/me", false},
		{"", "GET /api/me", false},
		{FullAccess, "POST /api/domains", true},
		{Session, "POST /api/domains", true},
		{SendingAccess, "POST /api/domains", false},
		{SendingAccess, "GET /api/domains", false},
		{FullAccess, "DELETE /api/suppressions/{id}", true},
		{SendingAccess, "DELETE /api/suppressions/{id}", false},
		{SendingAccess, "POST /api/suppressions/import", false},
		{SendingAccess, "POST /api/transactional", true},
		{SendingAccess, "GET /api/transactional/{id}", true},
		{SendingAccess, "GET /api/transactional", false},
		{SendingAccess, "GET /api/transactional/stats", false},
		{FullAccess, "GET /api/transactional/stats", true},
	}
	for _, c := range cases {
		if got := Allowed(c.scope, c.route); got != c.want {
			t.Errorf("Allowed(%q, %q) = %v, want %v", c.scope, c.route, got, c.want)
		}
	}
}
