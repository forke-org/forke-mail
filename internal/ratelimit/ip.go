// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package ratelimit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type IPResolver struct {
	TrustedProxy netip.Prefix
}

func (x IPResolver) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if !x.TrustedProxy.IsValid() || !x.TrustedProxy.Contains(remote.Unmap()) {
		return remote.Unmap().String()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if forwarded, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1])); err == nil {
		return forwarded.Unmap().String()
	}
	return remote.Unmap().String()
}
