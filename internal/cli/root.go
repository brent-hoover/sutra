package cli

import (
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

// Execute builds and runs the root command.
func Execute() error {
	cfg := config.Load()

	root := &cobra.Command{
		Use:           "sutra",
		Short:         "Sutra — an issue tracker for agent-assisted development",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		serveCmd(cfg),
		createCmd(cfg),
		viewCmd(cfg),
	)
	return root.Execute()
}
