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
	// Faint draws secondary text (Muted and what derives from it) in the
	// terminal's default color with the Faint attribute instead of ANSI 8,
	// which equals the background in some palettes (SPEC §9.2).
	Faint bool

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

// Theme names (config key theme).
const (
	ThemeTerminal      = "terminal"
	ThemeTerminalFaint = "terminal-faint"
)

// TerminalFaintTheme is TerminalTheme with Faint instead of ANSI 8 for
// secondary text. On a dark background the border is ANSI 0 (base02 in
// Solarized), which differs from the background where ANSI 8 does not.
func TerminalFaintTheme(isDark bool) Theme {
	t := TerminalTheme(isDark)
	t.Name = ThemeTerminalFaint
	t.Faint = true
	t.Muted = lipgloss.NoColor{}
	t.Status.Canceled = lipgloss.NoColor{}
	if isDark {
		t.Border = lipgloss.Black
	}
	return t
}

// ThemeByName returns the built-in theme with the given config name;
// unknown names give the default terminal theme.
func ThemeByName(name string, isDark bool) Theme {
	if name == ThemeTerminalFaint {
		return TerminalFaintTheme(isDark)
	}
	return TerminalTheme(isDark)
}
