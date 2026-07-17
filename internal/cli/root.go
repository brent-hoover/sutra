package cli

import (
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

// NewRoot builds the root command tree over the given config. Exposed so
// tests can drive the real commands with controlled args and output.
func NewRoot(cfg config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:           "sutra",
		Short:         "Sutra — an issue tracker for agent-assisted development",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", false, "print the raw JSON API response")
	root.AddCommand(
		serveCmd(cfg),
		createCmd(cfg),
		viewCmd(cfg),
	)
	return root
}

// Execute builds and runs the root command from process configuration.
func Execute() error {
	return NewRoot(config.Load()).Execute()
}
