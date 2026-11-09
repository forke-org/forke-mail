package auth

import (
	"net/http"
	"slices"

	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
)

const (
	SuperAdmin    = "super_admin"
	Admin         = "admin"
	FullAccess    = "full_access"
	SendingAccess = "sending_access"
)

var (
	anyone         = []string{SuperAdmin, Admin, FullAccess, SendingAccess}
	workspace      = []string{SuperAdmin, Admin, FullAccess}
	manage         = []string{SuperAdmin, FullAccess}
	sessions       = []string{SuperAdmin, Admin}
	superAdminOnly = []string{SuperAdmin}
)

var routeScopes = map[string][]string{
	"GET /api/me":                                              anyone,
	"POST /api/emails":                                         anyone,
	"POST /api/auth/logout":                                    sessions,
	"GET /api/keys":                                            superAdminOnly,
	"POST /api/keys":                                           superAdminOnly,
	"DELETE /api/keys/{id}":                                    superAdminOnly,
	"POST /api/keys/{id}/rotate":                               superAdminOnly,
	"GET /api/dashboard":                                       workspace,
	"GET /api/events":                                          workspace,
	"GET /api/contacts":                                        workspace,
	"POST /api/contacts/import":                                anyone,
	"POST /api/contacts/tags":                                  workspace,
	"DELETE /api/contacts/{id}":                                workspace,
	"GET /api/tags":                                            workspace,
	"POST /api/tags":                                           workspace,
	"GET /api/campaigns":                                       anyone,
	"POST /api/campaigns":                                      workspace,
	"GET /api/campaigns/{id}":                                  workspace,
	"PATCH /api/campaigns/{id}":                                workspace,
	"POST /api/campaigns/{id}/emails":                          anyone,
	"GET /api/emails/{id}":                                     anyone,
	"PATCH /api/emails/{id}":                                   workspace,
	"DELETE /api/emails/{id}":                                  workspace,
	"POST /api/emails/{id}/send":                               anyone,
	"POST /api/emails/{id}/pause":                              workspace,
	"POST /api/emails/{id}/resume":                             workspace,
	"GET /api/emails/{id}/report":                              anyone,
	"GET /api/emails/{id}/messages":                            workspace,
	"GET /api/emails/{id}/recipients":                          workspace,
	"GET /api/domains":                                         workspace,
	"POST /api/domains":                                        manage,
	"POST /api/domains/{id}/verify":                            workspace,
	"GET /api/domains/{id}/dns":                                workspace,
	"POST /api/webhooks":                                       manage,
	"GET /api/webhooks":                                        workspace,
	"PATCH /api/webhooks/{id}":                                 manage,
	"DELETE /api/webhooks/{id}":                                manage,
	"POST /api/webhooks/{id}/rotate":                           manage,
	"GET /api/webhooks/{id}/deliveries":                        workspace,
	"DELETE /api/domains/{id}":                                 manage,
	"GET /api/suppressions":                                    workspace,
	"POST /api/suppressions":                                   workspace,
	"POST /api/suppressions/import":                            workspace,
	"DELETE /api/suppressions/{id}":                            manage,
	"GET /api/settings/reengagement":                           workspace,
	"PUT /api/settings/reengagement":                           manage,
	"GET /api/topics":                                          workspace,
	"PUT /api/topics/{key}":                                    workspace,
	"POST /api/forke-mail/emails":                              anyone,
	"GET /api/forke-mail/emails/{id}":                          anyone,
	"GET /api/forke-mail/audiences":                            workspace,
	"POST /api/forke-mail/audiences":                           workspace,
	"POST /api/forke-mail/audiences/{id}/contacts":             workspace,
	"DELETE /api/forke-mail/audiences/{id}/contacts/{contact}": workspace,
	"POST /api/forke-mail/broadcasts":                          workspace,
	"POST /api/forke-mail/broadcasts/{id}/send":                workspace,
	"POST /api/resend/emails":                                  anyone,
	"GET /api/resend/emails/{id}":                              anyone,
	"GET /api/resend/audiences":                                workspace,
	"POST /api/resend/audiences":                               workspace,
	"POST /api/resend/audiences/{id}/contacts":                 workspace,
	"DELETE /api/resend/audiences/{id}/contacts/{contact}":     workspace,
	"POST /api/resend/broadcasts":                              workspace,
	"POST /api/resend/broadcasts/{id}/send":                    workspace,
	"POST /api/transactional":                                  anyone,
	"GET /api/transactional":                                   workspace,
	"GET /api/transactional/stats":                             workspace,
	"GET /api/transactional/{id}":                              anyone,
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
