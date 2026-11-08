package auth

import "testing"

func TestDeveloperManagementScopes(t *testing.T) {
	for _, route := range []string{"POST /api/webhooks", "GET /api/webhooks", "PATCH /api/webhooks/{id}", "DELETE /api/webhooks/{id}", "POST /api/webhooks/{id}/rotate", "GET /api/webhooks/{id}/deliveries", "GET /api/domains/{id}/dns"} {
		if !Allowed(Session, route) || !Allowed(FullAccess, route) || Allowed(SendingAccess, route) {
			t.Fatalf("incorrect scopes: %s", route)
		}
	}
}
