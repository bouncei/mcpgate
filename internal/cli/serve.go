package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bouncei/mcpgate/internal/config"
	"github.com/bouncei/mcpgate/internal/server"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the gateway",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			s, err := server.New(cfg)
			if err != nil {
				return err
			}
			httpSrv := &http.Server{
				Addr:    cfg.Listen,
				Handler: s.Handler(),
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				fmt.Fprintf(cmd.OutOrStdout(), "mcpgate listening on %s -> %s\n", cfg.Listen, cfg.Upstream.URL)
				if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					fmt.Fprintln(os.Stderr, "server error:", err)
					stop()
				}
			}()

			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return httpSrv.Shutdown(shutdownCtx)
		},
	}
	cmd.Flags().StringVarP(&path, "config", "c", "config.yaml", "path to config file")
	return cmd
}
