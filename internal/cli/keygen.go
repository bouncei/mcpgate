package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/bouncei/mcpgate/internal/auth"
	"github.com/spf13/cobra"
)

// generateKey returns a fresh random API key and its SHA-256 hash.
func generateKey() (key, hash string) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	key = "mcpg_" + hex.EncodeToString(raw)
	return key, auth.HashKey(key)
}

func newKeygenCmd() *cobra.Command {
	var label string
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate a new API key and its config entry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, hash := generateKey()
			if label == "" {
				label = "unnamed"
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "API key (give to the client; shown once):\n  %s\n\n", key)
			fmt.Fprintf(out, "Config entry (paste under keys: in config.yaml):\n")
			fmt.Fprintf(out, "  - label: %q\n    hash: %q\n    allow: []\n", label, hash)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "label for the key")
	return cmd
}
