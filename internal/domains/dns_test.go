package domains

import (
	"context"
	"net"
	"testing"
)

type fakeDNS struct {
	txt   map[string][]string
	cname string
	mx    []*net.MX
	err   error
}

func (d fakeDNS) LookupTXT(_ context.Context, n string) ([]string, error) { return d.txt[n], d.err }
func (d fakeDNS) LookupCNAME(context.Context, string) (string, error)     { return d.cname, d.err }
func (d fakeDNS) LookupMX(context.Context, string) ([]*net.MX, error)     { return d.mx, d.err }

func TestDNSStatuses(t *testing.T) {
	d := fakeDNS{txt: map[string][]string{"_dmarc.example.com": {"v=DMARC1; p=reject;"}, "bounce.example.com": {"v=spf1 include:amazonses.com ~all"}}, cname: "token.dkim.amazonses.com.", mx: []*net.MX{{Host: "feedback-smtp.us-east-1.amazonses.com.", Pref: 10}}}
	r := CheckDNS(context.Background(), d, "example.com", []string{"token"}, "bounce.example.com", "us-east-1")
	for _, c := range r.Checks {
		if c.Status != "found" {
			t.Fatalf("%s: %s", c.Name, c.Status)
		}
	}
	if !r.ConfigurationRequired || r.Mode != "custom_dns_preview" {
		t.Fatal("DNS preview claimed SES configuration")
	}
	d.txt["bounce.example.com"] = []string{"v=spf1 include:amazonses.com ~all", "v=spf1 -all"}
	d.cname = "wrong.example.com"
	r = CheckDNS(context.Background(), d, "example.com", []string{"token"}, "bounce.example.com", "us-east-1")
	if r.Checks[0].Status != "mismatch" || r.Checks[1].Status != "mismatch" {
		t.Fatal("accepted incorrect records")
	}
	r = CheckDNS(context.Background(), fakeDNS{err: &net.DNSError{IsNotFound: true}}, "example.com", []string{"token"}, "", "")
	for _, c := range r.Checks {
		if c.Status != "pending" {
			t.Fatal("missing DNS not pending")
		}
	}
	if r.Mode != "ses_default" || r.ConfigurationRequired || len(r.Checks) != 2 {
		t.Fatal("default mode incorrectly required custom SPF")
	}
}
