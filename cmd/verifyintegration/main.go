package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type verifier struct {
	dir     string
	failed  int
	results []checkResult
}

type checkResult struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL verification:", err)
		os.Exit(1)
	}
}

func output(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

func run() (runErr error) {
	branch, err := output("git", "branch", "--show-current")
	if err != nil || branch != "krishna" {
		return errors.New("current branch must be krishna")
	}
	if b, err := output("git", "fetch", "origin"); err != nil {
		return fmt.Errorf("git fetch failed: %s", b)
	}
	for _, ref := range []struct{ name, env, want string }{{"origin/advik", "VERIFY_EXPECTED_ADVIK", "a7d4f0c626369a3241aa168d241cdb2499e0f08d"}, {"origin/diya", "VERIFY_EXPECTED_DIYA", "f704c79db0313f15b81c86986653fb284accaf94"}} {
		want := os.Getenv(ref.env)
		if want == "" {
			want = ref.want
		}
		got, err := output("git", "rev-parse", ref.name)
		if err != nil || got != want {
			return fmt.Errorf("remote hash mismatch for %s: %s", ref.name, got)
		}
	}
	fmt.Println("PASS 1 branch and fetched teammate hashes match")
	adminURL := os.Getenv("VERIFY_DATABASE_URL")
	cfg, err := safeConfig(adminURL)
	if err != nil {
		return err
	}
	nonce := make([]byte, 6)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	name := "verify_" + time.Now().UTC().Format("20060102_150405") + "_" + hex.EncodeToString(nonce)
	dir := os.Getenv("VERIFY_EVIDENCE_DIR")
	if dir == "" {
		dir = filepath.Join("docs", "evidence", name)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	v := &verifier{dir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("local throwaway admin connection failed: %w", err)
	}
	defer admin.Close(context.Background())
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("create disposable DB: %w", err)
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 20*time.Second)
		defer c()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			fmt.Println("FAIL cleanup:", err)
			runErr = errors.Join(runErr, fmt.Errorf("throwaway DB cleanup failed: %w", err))
		} else {
			fmt.Println("PASS cleanup dropped", name)
		}
	}()
	child := url.URL{Scheme: "postgres", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), Path: "/" + name, User: url.UserPassword(cfg.User, cfg.Password), RawQuery: "sslmode=disable"}
	if _, err := safeConfig(child.String()); err != nil {
		return err
	}
	os.Setenv("TEST_DATABASE_URL", child.String())
	os.Setenv("GOMAXPROCS", "1")
	os.Setenv("GOFLAGS", "-p=1")
	os.Setenv("GOMEMLIMIT", "256MiB")
	os.Setenv("GOGC", "20")
	v.check("3 unique migration versions", uniqueMigrations)
	v.check("4 migration rollback and reapply", func() error {
		return v.tests("migrations", "./internal/db", "TestDeveloperMigrationsRollbackReapply")
	})
	v.check("5 live local HTTP cross-route idempotency and signed fake-receiver webhook", func() error {
		return v.tests("http-contract", "./cmd/server", "TestVerificationLiveHTTPContract")
	})
	v.check("6 sending key denied campaign creation", func() error {
		return v.tests("scopes", "./cmd/server", "TestIntegratedTenantIsolationAndSendingScopes")
	})
	v.check("7 no runtime stub or duplicate route registrations", sourceChecks)
	v.check("8a build", func() error { return v.command("build", "go", "build", "./...") })
	v.check("8b vet", func() error { return v.command("vet", "go", "vet", "./...") })
	v.check("8c gofmt", func() error {
		b, e := output("gofmt", "-l", "cmd", "internal", "migrations", "frontend")
		os.WriteFile(filepath.Join(dir, "gofmt.txt"), []byte(b), 0600)
		if e != nil {
			return e
		}
		if b != "" {
			return errors.New(b)
		}
		return nil
	})
	v.check("8d deadcode", func() error {
		if e := v.command("deadcode", "go", "run", "golang.org/x/tools/cmd/deadcode@v0.43.0", "-test", "./..."); e != nil {
			return e
		}
		b, e := os.ReadFile(filepath.Join(dir, "deadcode.txt"))
		if e == nil && len(strings.TrimSpace(string(b))) > 0 {
			return errors.New("deadcode reported unused code; see deadcode.txt")
		}
		return e
	})
	v.check("8e full normal tests", func() error { return v.command("tests", "go", "test", "-json", "-count=1", "./...") })
	if reason := raceToolchain(); reason != "" {
		fmt.Println("SKIPPED 8f race:", reason)
		v.results = append(v.results, checkResult{Check: "8f full race tests", Status: "SKIPPED", Reason: reason})
		os.WriteFile(filepath.Join(dir, "race.txt"), []byte("SKIPPED: "+reason), 0600)
	} else {
		v.check("8f full race tests", func() error { return v.command("race", "go", "test", "-race", "-json", "-count=1", "./...") })
	}
	summary, _ := json.MarshalIndent(map[string]any{"environment": "local (log driver/fake provider)", "database": name, "failures": v.failed, "checks": v.results}, "", "  ")
	os.WriteFile(filepath.Join(dir, "summary.json"), summary, 0600)
	if v.failed > 0 {
		return fmt.Errorf("%d checks failed; raw logs: %s", v.failed, dir)
	}
	fmt.Println("PASS all required checks; evidence:", dir)
	return nil
}

func safeConfig(raw string) (*pgx.ConnConfig, error) {
	if raw == "" {
		return nil, errors.New("set VERIFY_DATABASE_URL to a local verify_-prefixed admin DB with CREATEDB rights")
	}
	c, err := pgx.ParseConfig(raw)
	if err != nil {
		return nil, errors.New("invalid VERIFY_DATABASE_URL")
	}
	local := func(h string) bool { return h == "127.0.0.1" || h == "::1" || strings.EqualFold(h, "localhost") }
	if !local(c.Host) || !strings.HasPrefix(c.Database, "verify_") {
		return nil, errors.New("refusing DB: require literal local host and verify_ database prefix")
	}
	for _, f := range c.Fallbacks {
		if !local(f.Host) {
			return nil, errors.New("refusing non-local DB fallback")
		}
	}
	if strings.EqualFold(c.Host, "localhost") {
		c.Host = "127.0.0.1"
	}
	for _, f := range c.Fallbacks {
		if strings.EqualFold(f.Host, "localhost") {
			f.Host = "127.0.0.1"
		}
	}
	return c, nil
}

func (v *verifier) check(label string, f func() error) {
	if err := f(); err != nil {
		v.failed++
		v.results = append(v.results, checkResult{Check: label, Status: "FAIL", Reason: err.Error()})
		fmt.Println("FAIL", label+":", err)
	} else {
		v.results = append(v.results, checkResult{Check: label, Status: "PASS"})
		fmt.Println("PASS", label)
	}
}

func (v *verifier) command(log, name string, args ...string) error {
	file, err := os.Create(filepath.Join(v.dir, log+".txt"))
	if err != nil {
		return err
	}
	defer file.Close()
	cmd := exec.Command(name, args...)
	cmd.Stdout = file
	cmd.Stderr = file
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed; see %s.txt", name, log)
	}
	return nil
}

func raceToolchain() string {
	cc := os.Getenv("VERIFY_CC")
	if cc == "" {
		cc = os.Getenv("CC")
	}
	if cc == "" && runtime.GOOS == "windows" {
		for _, p := range []string{"C:/msys64/ucrt64/bin/gcc.exe", "C:/msys64/mingw64/bin/gcc.exe"} {
			if _, e := os.Stat(p); e == nil {
				cc = p
				break
			}
		}
	}
	if cc == "" {
		cc = "gcc"
	}
	if _, err := exec.LookPath(cc); err != nil {
		return "C compiler unavailable; set VERIFY_CC to a working 64-bit compiler"
	}
	if runtime.GOOS == "windows" {
		b, e := output(cc, "-dumpmachine")
		if e != nil || !strings.Contains(b, "x86_64") {
			return "selected Windows C compiler is not x86_64: " + b
		}
	}
	os.Setenv("CC", cc)
	os.Setenv("CGO_ENABLED", "1")
	os.Setenv("PATH", filepath.Dir(cc)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return ""
}
