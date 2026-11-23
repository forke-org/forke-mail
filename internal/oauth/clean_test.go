// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package oauth

import (
	"testing"

	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
)

func TestCleanRemovesStaleClients(t *testing.T) {
	pool := dbtest.Pool(t)
	stale, fresh := "fmc_"+auth.RandomHex(16), "fmc_"+auth.RandomHex(16)
	dbtest.Exec(t, pool, `INSERT INTO oauth_clients (id, name, redirect_uris, created_at) VALUES ($1, 'old', '{https://a.test/cb}', now() - interval '40 days'), ($2, 'new', '{https://a.test/cb}', now())`, stale, fresh)
	t.Cleanup(func() { dbtest.Exec(t, pool, `DELETE FROM oauth_clients WHERE id IN ($1, $2)`, stale, fresh) })
	if err := (&Server{Pool: pool}).Clean(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := dbtest.QueryString(t, pool, `SELECT string_agg(name, ',') FROM oauth_clients WHERE id IN ($1, $2)`, stale, fresh); got != "new" {
		t.Fatalf("remaining clients %q", got)
	}
}
