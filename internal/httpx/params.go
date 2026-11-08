package httpx

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidID(s string) bool {
	return uuidPattern.MatchString(s)
}

func ValidIDs(ids []string) bool {
	for _, id := range ids {
		if !ValidID(id) {
			return false
		}
	}
	return true
}

func PathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !ValidID(id) {
		Error(w, http.StatusNotFound, "not_found")
		return "", false
	}
	return id, true
}

func Page(r *http.Request) (limit, offset int) {
	limit = intParam(r, "limit", 50)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset = max(intParam(r, "offset", 0), 0)
	return limit, offset
}

func intParam(r *http.Request, name string, fallback int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return v
}
