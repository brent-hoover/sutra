package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brent-hoover/sutra/internal/client"
)

// Run launches the interactive TUI against the daemon reachable through c,
// blocking until the user quits or ctx is cancelled.
func Run(ctx context.Context, c *client.Client) error {
	p := tea.NewProgram(New(ctx, c), tea.WithContext(ctx), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
