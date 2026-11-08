package contacts

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
)

const maxCSVBytes = 5 << 20

func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	parsed, ok := parseUpload(w, r)
	if !ok {
		return
	}
	target := ImportTarget{EmailID: strings.TrimSpace(r.FormValue("email_id"))}
	if target.EmailID != "" && !httpx.ValidID(target.EmailID) {
		httpx.Error(w, http.StatusNotFound, "email_not_found")
		return
	}
	tags, err := normalizeTags(strings.Split(r.FormValue("tags"), ","))
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_tags")
		return
	}
	target.Tags = tags
	res, err := Import(r.Context(), h.Pool, ws, parsed, target)
	if errors.Is(err, errEmailNotFound) {
		httpx.Error(w, http.StatusNotFound, "email_not_found")
		return
	}
	if errors.Is(err, errEmailNotDraft) {
		httpx.Error(w, http.StatusConflict, "email_not_draft")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func parseUpload(w http.ResponseWriter, r *http.Request) (Parsed, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBytes)
	if err := r.ParseMultipartForm(maxCSVBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "body_too_large")
			return Parsed{}, false
		}
		httpx.Error(w, http.StatusBadRequest, "invalid_multipart")
		return Parsed{}, false
	}
	defer r.MultipartForm.RemoveAll()

	var src io.Reader
	if f, _, err := r.FormFile("file"); err == nil {
		defer f.Close()
		src = f
	} else if text := r.FormValue("text"); strings.TrimSpace(text) != "" {
		src = strings.NewReader(text)
	} else {
		httpx.Error(w, http.StatusBadRequest, "missing_csv")
		return Parsed{}, false
	}
	parsed, err := ParseCSV(src)
	if errors.Is(err, errTooManyRows) {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_many_rows")
		return Parsed{}, false
	}
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_csv")
		return Parsed{}, false
	}
	return parsed, true
}
