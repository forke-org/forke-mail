package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type logFieldsKey struct{}

type logFields struct {
	workspaceID string
	keyPrefix   string
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

func SetLogFields(ctx context.Context, workspaceID, keyPrefix string) {
	if f, ok := ctx.Value(logFieldsKey{}).(*logFields); ok {
		f.workspaceID = workspaceID
		f.keyPrefix = keyPrefix
	}
}

func RoutePath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return "unmatched"
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fields := &logFields{}
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), logFieldsKey{}, fields)))
		slog.Info("request",
			"method", r.Method,
			"path", RoutePath(r),
			"status", max(rec.status, http.StatusOK),
			"duration_ms", time.Since(start).Milliseconds(),
			"workspace_id", fields.workspaceID,
			"key_prefix", fields.keyPrefix,
		)
	})
}
