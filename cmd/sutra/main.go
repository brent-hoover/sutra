// Command sutra is the single binary: `sutra serve` runs the daemon, `sutra`
// with no subcommand launches the TUI, and the other subcommands are the CLI.
// It is the composition root.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/brent-hoover/sutra/internal/cli"
	"github.com/brent-hoover/sutra/internal/client"
	"github.com/brent-hoover/sutra/internal/config"
	"github.com/brent-hoover/sutra/internal/tui"
)

func main() {
	// Inject the TUI launcher so the cli package need not import tui (arch rule).
	cli.TUILauncher = func(ctx context.Context, cfg config.Config) error {
		return tui.Run(ctx, client.New(cfg))
	}
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
