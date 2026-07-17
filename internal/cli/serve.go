package cli

import (
	"fmt"

	"github.com/brent-hoover/sutra/internal/api"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

func serveCmd(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the Sutra daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.ErrOrStderr(), "sutra serve: listening on %s (db %s)\n", cfg.ListenAddr, cfg.DBPath)
			return api.Run(cfg)
		},
	}
}
