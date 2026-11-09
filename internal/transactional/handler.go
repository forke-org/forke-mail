package transactional

import (
	"errors"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"net/http"
	"strings"
	"unicode/utf8"
)

type CompatibilityHandler struct{ Service Service }
type Receipt struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
}
type compatibilityRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

func (h *CompatibilityHandler) Send(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 || !validIdempotencyKey(key) {
		httpx.Error(w, 400, "invalid_idempotency_key")
		return
	}
	var body compatibilityRequest
	if !httpx.Decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Subject) == "" || utf8.RuneCountInString(body.Subject) > 256 {
		httpx.Error(w, 422, "invalid_email_request")
		return
	}
	res, err := h.Service.Accept(r.Context(), auth.FromContext(r.Context()), key, Submission{FromAddress: body.From, To: body.To, Subject: body.Subject, HTML: body.HTML, Text: body.Text})
	switch {
	case errors.Is(err, errKeyReused):
		httpx.Error(w, 409, "idempotency_conflict")
	case errors.Is(err, errDomainUnverified):
		httpx.Error(w, 422, "sender_not_verified")
	case isValidation(err):
		v := err.(*validationError)
		code := v.code
		if v.status == 422 {
			code = "invalid_email_request"
		}
		httpx.Error(w, v.status, code)
	case writeError(w, r, err):
		return
	default:
		w.Header().Set("Cache-Control", "no-store")
		if res.Replayed != "" {
			w.Header().Set("Idempotency-Replayed", "true")
		}
		httpx.JSON(w, 202, Receipt{ID: res.ID, Status: "queued", Mode: "engine"})
	}
}
