package resendapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"forkemail/internal/emails"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool          *pgxpool.Pool
	Transactional TransactionalAPI
	Emails        *emails.Handler
	Tags          http.HandlerFunc
}

type TransactionalAPI interface {
	Send(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
}

type capture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capture) Header() http.Header         { return c.header }
func (c *capture) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *capture) WriteHeader(status int)      { c.status = status }

func (c *capture) ok() bool { return c.status >= 200 && c.status < 300 }

func (c *capture) code() string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(c.body.Bytes(), &e)
	return e.Error
}

func delegate(h http.HandlerFunc, r *http.Request, body any, pathID string) *capture {
	inner := r.Clone(r.Context())
	if body != nil {
		b, _ := json.Marshal(body)
		inner.Body = io.NopCloser(bytes.NewReader(b))
		inner.ContentLength = int64(len(b))
	}
	if pathID != "" {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", pathID)
		inner = inner.WithContext(context.WithValue(inner.Context(), chi.RouteCtxKey, rctx))
	}
	c := &capture{header: http.Header{}, status: http.StatusOK}
	h(c, inner)
	return c
}

func relay(w http.ResponseWriter, c *capture) {
	for _, name := range []string{"Retry-After", "Idempotent-Replayed"} {
		if v := c.header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	fail(w, c.status, c.code())
}

func fail(w http.ResponseWriter, status int, message string) {
	httpx.JSON(w, status, map[string]any{"statusCode": status, "name": errorName(status), "message": message})
}

func errorName(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "validation_error"
	case http.StatusUnauthorized:
		return "missing_api_key"
	case http.StatusForbidden:
		return "restricted_api_key"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "invalid_idempotent_request"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusPaymentRequired:
		return "daily_quota_exceeded"
	}
	return "application_error"
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusUnprocessableEntity, "invalid_json")
		return false
	}
	return true
}
