// Package common holds UI building blocks shared by screens: theme and
// styles, key bindings, footer, modals.
package common

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme maps semantic roles to colors. Screens never use colors directly;
// they use Styles built from a Theme (SPEC §9).
type Theme struct {
	Name   string
	IsDark bool

	Fg        color.Color // main text; NoColor = terminal default
	Muted     color.Color // secondary text, column headers
	Border    color.Color
	Accent    color.Color // app name, keys, active tab, selected IID
	Selection color.Color // background of the selected row; nil = reverse
	Warning   color.Color

	Status struct {
		Success, Failed, Running, Pending, Manual, Canceled color.Color
	}
	Diff struct {
		Add, Del color.Color
	}
}

// TerminalTheme uses only the 16 ANSI colors so the UI follows the user's
// terminal palette. Yellow is unreadable on most light palettes, so the
// accent switches to blue there.
func TerminalTheme(isDark bool) Theme {
	t := Theme{
		Name:   "terminal",
		IsDark: isDark,
		Fg:     lipgloss.NoColor{},
		Muted:  lipgloss.BrightBlack,
		Border: lipgloss.BrightBlack,
	}
	t.Status.Success = lipgloss.Green
	t.Status.Failed = lipgloss.Red
	t.Status.Running = lipgloss.Blue
	t.Status.Pending = lipgloss.Yellow
	t.Status.Manual = lipgloss.Magenta
	t.Status.Canceled = lipgloss.BrightBlack
	t.Diff.Add = lipgloss.Green
	t.Diff.Del = lipgloss.Red

	if isDark {
		t.Accent = lipgloss.Yellow
		t.Warning = lipgloss.Yellow
	} else {
		t.Accent = lipgloss.Blue
		t.Warning = lipgloss.Red
	}
	return t
}
