// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package smtpserver

import (
	"net"
	"net/http"

	"forkemail/internal/httpx"
)

func port(addr string) string {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return p
}

func (c Config) Settings(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"enabled": c.Enabled(), "host": c.Hostname, "username": Username,
		"starttls_port": port(c.Addr), "tls_port": port(c.TLSAddr),
	})
}
