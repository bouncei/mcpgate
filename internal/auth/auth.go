package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/bouncei/mcpgate/internal/config"
)

// Identity is the resolved principal for an authenticated request.
type Identity struct {
	Label     string
	Allow     []string
	RateLimit config.RateLimit
}

type Authenticator struct {
	keys map[string]*config.Key // keyed by lowercase SHA-256 hex hash
}

// HashKey returns the lowercase SHA-256 hex digest of a plaintext key.
// Keys are high-entropy random tokens, so a fast hash is appropriate.
func HashKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func New(cfg *config.Config) *Authenticator {
	a := &Authenticator{keys: make(map[string]*config.Key, len(cfg.Keys))}
	for i := range cfg.Keys {
		k := &cfg.Keys[i]
		a.keys[strings.ToLower(k.Hash)] = k
	}
	return a
}

// Authenticate resolves a presented plaintext key to an Identity.
func (a *Authenticator) Authenticate(presented string) (*Identity, bool) {
	if presented == "" {
		return nil, false
	}
	k, ok := a.keys[HashKey(presented)]
	if !ok {
		return nil, false
	}
	rl := config.RateLimit{}
	if k.RateLimit != nil {
		rl = *k.RateLimit
	}
	return &Identity{Label: k.Label, Allow: k.Allow, RateLimit: rl}, true
}
