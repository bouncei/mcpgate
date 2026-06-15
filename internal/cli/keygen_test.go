package cli

import (
	"strings"
	"testing"

	"github.com/bouncei/mcpgate/internal/auth"
)

func TestGenerateKeyMatchesHash(t *testing.T) {
	key, hash, err := generateKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "mcpg_") {
		t.Errorf("key prefix missing: %q", key)
	}
	if auth.HashKey(key) != hash {
		t.Error("printed hash does not match HashKey(key)")
	}
	k2, _, _ := generateKey()
	if key == k2 {
		t.Error("keys should be unique")
	}
}
