package auth

import (
	"net/http"
	"slices"

	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const (
	Session       = "session"
	FullAccess    = "full_access"
	SendingAccess = "sending_access"
)

var (
	anyone      = []string{Session, FullAccess, SendingAccess}
	workspace   = []string{Session, FullAccess}
	sessionOnly = []string{Session}
)

var routeScopes = map[string][]string{
	"GET /api/me":                     anyone,
	"POST /api/auth/logout":           sessionOnly,
	"GET /api/keys":                   sessionOnly,
	"POST /api/keys":                  sessionOnly,
	"DELETE /api/keys/{id}":           sessionOnly,
	"POST /api/keys/{id}/rotate":      sessionOnly,
	"GET /api/dashboard":              workspace,
	"GET /api/events":                 workspace,
	"GET /api/contacts":               workspace,
	"POST /api/contacts/import":       anyone,
	"POST /api/contacts/tags":         workspace,
	"DELETE /api/contacts/{id}":       workspace,
	"GET /api/tags":                   workspace,
	"POST /api/tags":                  workspace,
	"GET /api/campaigns":              anyone,
	"POST /api/campaigns":             workspace,
	"GET /api/campaigns/{id}":         workspace,
	"PATCH /api/campaigns/{id}":       workspace,
	"POST /api/campaigns/{id}/emails": anyone,
	"GET /api/emails/{id}":            anyone,
	"PATCH /api/emails/{id}":          workspace,
	"DELETE /api/emails/{id}":         workspace,
	"POST /api/emails/{id}/send":      anyone,
	"POST /api/emails/{id}/pause":     workspace,
	"POST /api/emails/{id}/resume":    workspace,
	"GET /api/emails/{id}/report":     anyone,
	"GET /api/emails/{id}/messages":   workspace,
	"GET /api/emails/{id}/recipients": workspace,
	"GET /api/domains":                workspace,
	"POST /api/domains":               workspace,
	"POST /api/domains/{id}/verify":   workspace,
	"GET /api/domains/{id}/dns":       workspace,
	"DELETE /api/domains/{id}":        workspace,
	"GET /api/suppressions":           workspace,
	"POST /api/suppressions":          workspace,
	"POST /api/suppressions/import":   workspace,
	"DELETE /api/suppressions/{id}":   workspace,
}

func Allowed(scope, route string) bool {
	return slices.Contains(routeScopes[route], scope)
}

func Authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pattern := chi.RouteContext(r.Context()).Routes.Find(chi.NewRouteContext(), r.Method, r.URL.Path)
		if pattern == "" {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		if !Allowed(FromContext(r.Context()).Scope, r.Method+" "+pattern) {
			httpx.Error(w, http.StatusForbidden, "insufficient_scope")
			return
		}
		next.ServeHTTP(w, r)
	})
}
