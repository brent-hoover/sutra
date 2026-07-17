// Command sutra is the single binary: `sutra serve` runs the daemon, and the
// other subcommands are the CLI. It is the composition root.
package main

import (
	"fmt"
	"os"

	"github.com/brent-hoover/sutra/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
