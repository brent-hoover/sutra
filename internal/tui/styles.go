package tui

import "github.com/charmbracelet/lipgloss"

// styles holds the Lipgloss styles the views share. Kept in one place so the
// look is consistent and easy to change.
var styles = struct {
	title     lipgloss.Style
	help      lipgloss.Style
	status    lipgloss.Style
	errMsg    lipgloss.Style
	cursor    lipgloss.Style
	selected  lipgloss.Style
	dim       lipgloss.Style
	label     lipgloss.Style
	section   lipgloss.Style
	fieldName lipgloss.Style
	active    lipgloss.Style
}{
	title:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
	help:      lipgloss.NewStyle().Faint(true),
	status:    lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
	errMsg:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
	cursor:    lipgloss.NewStyle().Foreground(lipgloss.Color("205")),
	selected:  lipgloss.NewStyle().Bold(true),
	dim:       lipgloss.NewStyle().Faint(true),
	label:     lipgloss.NewStyle().Bold(true),
	section:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
	fieldName: lipgloss.NewStyle().Bold(true).Width(10),
	active:    lipgloss.NewStyle().Foreground(lipgloss.Color("205")),
}
