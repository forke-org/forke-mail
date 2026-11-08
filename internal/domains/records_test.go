package domains

import (
	"context"
	"reflect"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestDNSRecords(t *testing.T) {
	got := DNSRecords("domain", "mail.example.com", []string{"aaa", "bbb", "ccc"})
	want := []Record{
		{"CNAME", "aaa._domainkey.mail.example.com", "aaa.dkim.amazonses.com"},
		{"CNAME", "bbb._domainkey.mail.example.com", "bbb.dkim.amazonses.com"},
		{"CNAME", "ccc._domainkey.mail.example.com", "ccc.dkim.amazonses.com"},
		{"TXT", "mail.example.com", "v=spf1 include:amazonses.com ~all"},
		{"TXT", "_dmarc.mail.example.com", "v=DMARC1; p=none;"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if got := DNSRecords("email", "me@example.com", nil); len(got) != 0 {
		t.Fatalf("email identity records %+v", got)
	}
}

func TestValidName(t *testing.T) {
	valid := map[string]string{"example.com": "domain", "mail.forke.space": "domain", "a-b.co.uk": "domain", "me@example.com": "email"}
	invalid := map[string]string{
		"localhost": "domain", "-bad.com": "domain", "bad-.com": "domain", "exa mple.com": "domain", "a..com": "domain",
		"1.2.3.4": "domain", "example.com.": "domain", "me@example.com": "domain", "example.com": "email", "x": "other",
	}
	for name, kind := range valid {
		if !ValidName(kind, name) {
			t.Errorf("%s %s rejected", kind, name)
		}
	}
	for name, kind := range invalid {
		if ValidName(kind, name) {
			t.Errorf("%s %s accepted", kind, name)
		}
	}
}

func TestIdentityUniqueAcrossWorkspaces(t *testing.T) {
	pool := dbtest.Pool(t)
	a := dbtest.Workspace(t, pool, "paid", 0)
	b := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO domains (workspace_id, name) VALUES ($1, 'shared-identity.example')`, a)
	if _, err := pool.Exec(context.Background(), `INSERT INTO domains (workspace_id, name) VALUES ($1, 'Shared-Identity.example')`, b); err == nil {
		t.Fatal("second workspace claimed the same identity")
	}
}
