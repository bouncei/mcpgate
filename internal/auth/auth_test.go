package auth

import (
	"testing"

	"github.com/bouncei/mcpgate/internal/config"
)

func testConfig() *config.Config {
	rl := config.RateLimit{RPS: 5, Burst: 10}
	return &config.Config{
		Keys: []config.Key{
			{Label: "alice", Hash: HashKey("secret-alice"), Allow: []string{"read_file"}, RateLimit: &rl},
			{Label: "bob", Hash: HashKey("secret-bob"), Allow: []string{"*"}, RateLimit: &rl},
		},
	}
}

func TestAuthenticateValid(t *testing.T) {
	a := New(testConfig())
	id, ok := a.Authenticate("secret-alice")
	if !ok {
		t.Fatal("expected alice to authenticate")
	}
	if id.Label != "alice" {
		t.Errorf("label = %q", id.Label)
	}
	if len(id.Allow) != 1 || id.Allow[0] != "read_file" {
		t.Errorf("allow = %v", id.Allow)
	}
}

func TestAuthenticateInvalid(t *testing.T) {
	a := New(testConfig())
	if _, ok := a.Authenticate("wrong"); ok {
		t.Fatal("expected wrong key to fail")
	}
	if _, ok := a.Authenticate(""); ok {
		t.Fatal("expected empty key to fail")
	}
}

func TestHashKeyDeterministic(t *testing.T) {
	if HashKey("x") != HashKey("x") {
		t.Fatal("hash not deterministic")
	}
	if HashKey("x") == HashKey("y") {
		t.Fatal("hash collision")
	}
}
