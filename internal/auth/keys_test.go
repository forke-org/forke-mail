// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	k := GenerateKey(ModeLive)
	if !strings.HasPrefix(k.Plain, "fm_live_") || len(k.Plain) != 72 {
		t.Fatalf("unexpected key format %q", k.Plain)
	}
	if !validKeyFormat(k.Plain) {
		t.Fatal("generated key fails format check")
	}
	if k.Prefix != k.Plain[:12] {
		t.Fatalf("prefix %q is not the first 12 characters", k.Prefix)
	}
	sum := sha256.Sum256([]byte(k.Plain))
	if k.Hash != hex.EncodeToString(sum[:]) || HashKey(k.Plain) != k.Hash {
		t.Fatal("hash is not the sha256 of the key")
	}
	if strings.Contains(k.Hash, k.Plain) {
		t.Fatal("hash contains the plain key")
	}
	if GenerateKey(ModeLive).Plain == k.Plain {
		t.Fatal("keys are not random")
	}
}

func TestValidKeyFormat(t *testing.T) {
	for _, bad := range []string{"", "fm_live_", "fm_prod_" + strings.Repeat("a", 64), "fm_live_" + strings.Repeat("A", 64), "fm_live_" + strings.Repeat("a", 63)} {
		if validKeyFormat(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, mode := range []string{ModeLive, ModeTest} {
		if k := GenerateKey(mode).Plain; !validKeyFormat(k) || !strings.HasPrefix(k, "fm_"+mode+"_") {
			t.Errorf("rejected %s key", mode)
		}
	}
}
