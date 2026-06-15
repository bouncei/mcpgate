package cli

import "github.com/spf13/cobra"

// version is overridden at build time via -ldflags.
var version = "dev"

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "mcpgate",
		Short: "Auth gateway for self-hosted MCP servers",
	}
	root.AddCommand(newKeygenCmd())
	root.AddCommand(newValidateCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println(version)
		},
	})
	return root
}
