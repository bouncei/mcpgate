package cli

import (
	"fmt"

	"github.com/bouncei/mcpgate/internal/config"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK: %d key(s), upstream %s\n", len(cfg.Keys), cfg.Upstream.URL)
			return nil
		},
	}
	cmd.Flags().StringVarP(&path, "config", "c", "config.yaml", "path to config file")
	return cmd
}
