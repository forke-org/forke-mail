package feedback

import (
	"io"
	"net/http"

	"forkemail/internal/httpx"
	"forkemail/internal/ratelimit"
)

type DevHandler struct {
	Processor *Processor
	Token     string
	Failures  ratelimit.FailureGuard
}

func (h *DevHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Failures.Reject(w, r) {
		return
	}
	if !httpx.TokenMatches(r.Header.Get("X-Seed-Token"), h.Token) {
		h.Failures.Fail(r)
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	result, err := h.Processor.Handle(r.Context(), "", body)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"result": result})
}
