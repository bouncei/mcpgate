package cli

import (
	"strings"
	"testing"

	"github.com/bouncei/mcpgate/internal/auth"
)

func TestGenerateKeyMatchesHash(t *testing.T) {
	key, hash := generateKey()
	if !strings.HasPrefix(key, "mcpg_") {
		t.Errorf("key prefix missing: %q", key)
	}
	if auth.HashKey(key) != hash {
		t.Error("printed hash does not match HashKey(key)")
	}
	k2, _ := generateKey()
	if key == k2 {
		t.Error("keys should be unique")
	}
}
