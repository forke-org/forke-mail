// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package frontend

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"sync"
)

//go:embed all:out
var out embed.FS

func Handler() http.Handler {
	sub, _ := fs.Sub(out, "out")
	return http.FileServer(http.FS(sub))
}

var (
	versionsOnce sync.Once
	versions     map[string]string
)

func Asset(path string) string {
	versionsOnce.Do(func() {
		versions = map[string]string{}
		_ = fs.WalkDir(out, "out", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.Contains(p, "/_next/") {
				return nil
			}
			b, err := out.ReadFile(p)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(b)
			versions[strings.TrimPrefix(p, "out")] = hex.EncodeToString(sum[:6])
			return nil
		})
	})
	if v, ok := versions[path]; ok {
		return path + "?v=" + v
	}
	return path
}
