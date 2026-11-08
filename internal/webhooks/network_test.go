package webhooks

import (
	"context"
	"net/http"
	"net/netip"
	"testing"
)

type fixedResolver struct{ ips []netip.Addr }

func (r fixedResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.ips, nil
}

func TestSSRFProtection(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "224.0.0.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2001:db8::1"} {
		if PublicIP(netip.MustParseAddr(s)) {
			t.Fatalf("allowed %s", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !PublicIP(netip.MustParseAddr(s)) {
			t.Fatalf("blocked public %s", s)
		}
	}
	n := Network{Resolver: fixedResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8")}}}
	if err := n.Validate(context.Background(), "https://hooks.example.com/event"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"http://hooks.example.com", "https://user:pass@hooks.example.com", "https://hooks.example.com:8080", "https://hooks.example.com/#fragment"} {
		if n.Validate(context.Background(), s) == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	n.Resolver = fixedResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}}
	if n.Validate(context.Background(), "https://hooks.example.com") == nil {
		t.Fatal("mixed DNS accepted")
	}
	client := n.Client()
	if err := client.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatal("redirects enabled")
	}
	transport := client.Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "hooks.example.com:443"); err == nil {
		t.Fatal("dial bypassed DNS validation")
	}
}

func TestDNSRebindingAtConnection(t *testing.T) {
	n := Network{Resolver: fixedResolver{[]netip.Addr{netip.MustParseAddr("8.8.8.8")}}}
	if err := n.Validate(context.Background(), "https://hooks.example.com"); err != nil {
		t.Fatal(err)
	}
	n.Resolver = fixedResolver{[]netip.Addr{netip.MustParseAddr("169.254.169.254")}}
	transport := n.Client().Transport.(*http.Transport)
	if _, err := transport.DialContext(context.Background(), "tcp", "hooks.example.com:443"); err == nil {
		t.Fatal("rebound DNS allowed")
	}
}
