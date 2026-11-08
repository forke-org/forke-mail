package webhooks

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type Network struct{ Resolver Resolver }

var denied = []netip.Prefix{
	netip.MustParsePrefix("168.63.129.16/32"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("::/96"), netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range denied {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("invalid_webhook_url")
	}
	return u, nil
}

func (n Network) addresses(ctx context.Context, host string) ([]netip.Addr, error) {
	resolver := n.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ips, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("webhook_dns_unavailable")
	}
	for _, ip := range ips {
		if !PublicIP(ip) {
			return nil, errors.New("unsafe_webhook_destination")
		}
	}
	return ips, nil
}

func (n Network) Validate(ctx context.Context, raw string) error {
	u, err := parseURL(raw)
	if err != nil {
		return err
	}
	_, err = n.addresses(ctx, u.Hostname())
	return err
}

func (n Network) Client() *http.Client {
	t := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || port != "443" {
				return nil, errors.New("unsafe_webhook_destination")
			}
			ips, err := n.addresses(ctx, host)
			if err != nil {
				return nil, err
			}
			var last error
			for _, ip := range ips {
				conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, last
		},
	}
	return &http.Client{Transport: t, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
