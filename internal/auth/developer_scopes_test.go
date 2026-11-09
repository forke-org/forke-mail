package auth

import "testing"

func TestDeveloperManagementScopes(t *testing.T) {
	for _, route := range []string{"POST /api/webhooks", "PATCH /api/webhooks/{id}", "DELETE /api/webhooks/{id}", "POST /api/webhooks/{id}/rotate"} {
		if !Allowed(SuperAdmin, route) || !Allowed(FullAccess, route) || Allowed(Admin, route) || Allowed(SendingAccess, route) {
			t.Fatalf("incorrect scopes: %s", route)
		}
	}
	for _, route := range []string{"GET /api/webhooks", "GET /api/webhooks/{id}/deliveries", "GET /api/domains/{id}/dns"} {
		if !Allowed(SuperAdmin, route) || !Allowed(Admin, route) || !Allowed(FullAccess, route) || Allowed(SendingAccess, route) {
			t.Fatalf("incorrect scopes: %s", route)
		}
	}
}
