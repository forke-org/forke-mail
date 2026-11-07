package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

const maxJSONBytes = 1 << 20

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, code string) {
	JSON(w, status, map[string]string{"error": code})
}

func Internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "path", RoutePath(r), "err", err)
	Error(w, http.StatusInternalServerError, "internal_error")
}

func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			Error(w, http.StatusRequestEntityTooLarge, "body_too_large")
			return false
		}
		Error(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		Error(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	return true
}
