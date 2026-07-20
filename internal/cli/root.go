package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/brent-hoover/sutra/internal/config"
	"github.com/spf13/cobra"
)

// TUILauncher launches the interactive TUI. It is injected by the composition
// root (cmd/sutra) so this package need not import internal/tui, which the
// architecture rules forbid (cli may import domain, client, api, config only).
var TUILauncher func(ctx context.Context, cfg config.Config) error

// NewRoot builds the root command tree over the given config. Exposed so
// tests can drive the real commands with controlled args and output.
//
// Running `sutra` with no subcommand launches the TUI (when a launcher has been
// injected); the subcommands are the CLI and the daemon.
func NewRoot(cfg config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:           "sutra",
		Short:         "Sutra — an issue tracker for agent-assisted development",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if TUILauncher == nil {
				return errors.New("TUI is not available in this build")
			}
			return TUILauncher(cmd.Context(), cfg)
		},
	}
	root.PersistentFlags().Bool("json", false, "print the raw JSON API response")
	root.AddCommand(
		serveCmd(cfg),
		createCmd(cfg),
		viewCmd(cfg),
		listCmd(cfg),
		updateCmd(cfg),
		deleteCmd(cfg),
		historyCmd(cfg),
		commentCmd(cfg),
		docCmd(cfg),
		transcriptCmd(cfg),
		linkCmd(cfg),
		labelCmd(cfg),
		searchCmd(cfg),
		activityCmd(cfg),
		projectCmd(cfg),
		threadCmd(cfg),
		skillCmd(cfg),
	)
	return root
}

// Execute builds and runs the root command from process configuration, under a
// context cancelled on SIGINT/SIGTERM so `serve` shuts down gracefully.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return NewRoot(cfg).ExecuteContext(ctx)
}
