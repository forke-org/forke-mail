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
