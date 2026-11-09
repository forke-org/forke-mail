package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerificationRejectsUnsafeDatabases(t *testing.T) {
	for _, raw := range []string{"", "postgres://u@db.example.com/verify_test", "postgres://u@127.0.0.1/production", "postgres://u@127.0.0.1/verify_admin?host=10.0.0.1", "postgres://u@127.0.0.1/verify_admin?dbname=production"} {
		if _, err := safeConfig(raw); err == nil {
			t.Fatal("unsafe database accepted")
		}
	}
	for _, raw := range []string{"postgres://u@127.0.0.1/verify_admin", "postgres://u@localhost/verify_admin", "postgres://u@[::1]/verify_admin"} {
		if _, err := safeConfig(raw); err != nil {
			t.Fatal("local throwaway refused", err)
		}
	}
}

func TestVerificationSourceAndMigrationChecks(t *testing.T) {
	t.Chdir("../..")
	if err := sourceChecks(); err != nil {
		t.Fatal(err)
	}
	if err := uniqueMigrations(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifierDoesNotAcceptMissingOrSkippedContractTests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tests.jsonl")
	for _, action := range []string{"skip", "run", "pass"} {
		if err := os.WriteFile(path, []byte(`{"Action":"`+action+`","Test":"Contract"}`), 0600); err != nil {
			t.Fatal(err)
		}
		err := requireTests(path, []string{"Contract"})
		if (err == nil) != (action == "pass") {
			t.Fatal("incorrect test proof", action, err)
		}
	}
	if err := requireTests(path, []string{"Missing"}); err == nil {
		t.Fatal("missing test accepted")
	}
}
