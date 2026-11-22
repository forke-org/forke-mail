// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package frontend

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:out
var out embed.FS

func Handler() http.Handler {
	sub, _ := fs.Sub(out, "out")
	return http.FileServer(http.FS(sub))
}
