package common

import (
	"image/color"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/gitlab"
)

// ToastLevel is the severity of a toast.
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarn
	ToastError
)

// Styles are ready-made lipgloss styles for every UI element. Screens hold a
// *Styles and receive a new pointer on ThemeChangedMsg.
type Styles struct {
	Theme Theme

	Text, Muted, Accent, Bold lipgloss.Style
	Success, Error, Warning   lipgloss.Style

	AppName, Header, HeaderMuted, Breadcrumb lipgloss.Style

	Tab, TabActive, TabCount lipgloss.Style

	FilterKey, FilterValue lipgloss.Style

	ColumnHeader, Row, RowSelected, RowMuted, Cursor lipgloss.Style
	GroupHeader, MoreRow                             lipgloss.Style

	Preview, PreviewTitle lipgloss.Style

	Footer, Key, KeyDesc lipgloss.Style

	Toast map[ToastLevel]lipgloss.Style

	Modal, ModalTitle, Button, ButtonActive lipgloss.Style

	LogLineNo, LogSection, LogMatch, LogMatchCurrent lipgloss.Style
	DiffAdd, DiffDel                                 lipgloss.Style

	// Rule is the style of thin separator lines; Chips and Boxes are
	// bordered containers (blocks.go).
	Rule  lipgloss.Style
	Chips map[ChipRole]lipgloss.Style
	Boxes map[BoxKind]lipgloss.Style

	// Input styles text inputs with theme colors only (no 256-color defaults).
	Input textinput.Styles

	// Markdown is the glamour style config for descriptions and comments.
	Markdown ansi.StyleConfig

	status map[gitlab.PipelineStatus]lipgloss.Style
}

// NewStyles builds Styles from a theme. Nil colors are treated as "no color".
func NewStyles(t Theme) *Styles {
	s := &Styles{Theme: t}

	s.Text = fg(t.Fg)
	s.Muted = fg(t.Muted)
	canceled := fg(t.Status.Canceled)
	if t.Faint {
		s.Muted = s.Muted.Faint(true)
		canceled = canceled.Faint(true)
	}
	s.Accent = fg(t.Accent)
	s.Bold = lipgloss.NewStyle().Bold(true)
	s.Success = fg(t.Status.Success)
	s.Error = fg(t.Status.Failed)
	s.Warning = fg(t.Warning)

	s.AppName = s.Accent.Bold(true)
	s.Header = s.Text
	s.HeaderMuted = s.Muted
	s.Breadcrumb = s.Muted

	s.Tab = s.Muted.Padding(0, 1)
	s.TabActive = s.Accent.Bold(true).Reverse(true).Padding(0, 1)
	s.TabCount = lipgloss.NewStyle()

	s.FilterKey = s.Muted
	s.FilterValue = s.Text

	s.ColumnHeader = s.Muted
	s.Row = s.Text
	s.RowSelected = lipgloss.NewStyle().Reverse(true)
	if t.Selection != nil {
		s.RowSelected = lipgloss.NewStyle().Background(t.Selection)
	}
	s.RowMuted = s.Muted
	s.Cursor = s.Accent.Bold(true)
	s.GroupHeader = s.Bold
	s.MoreRow = s.Muted

	s.Preview = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(t.Border).Padding(0, 1)
	s.PreviewTitle = s.Bold

	s.Footer = lipgloss.NewStyle()
	s.Key = s.Accent
	s.KeyDesc = s.Muted

	s.Toast = map[ToastLevel]lipgloss.Style{
		ToastInfo:    s.Text,
		ToastSuccess: s.Success,
		ToastWarn:    s.Warning,
		ToastError:   s.Error.Bold(true),
	}

	s.Modal = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).Padding(0, 1)
	s.ModalTitle = s.Bold
	s.Button = s.Muted.Padding(0, 1)
	s.ButtonActive = s.Accent.Bold(true).Reverse(true).Padding(0, 1)

	s.LogLineNo = s.Muted
	s.LogSection = s.Bold
	s.LogMatch = lipgloss.NewStyle().Underline(true)
	s.LogMatchCurrent = s.Accent.Reverse(true)
	s.DiffAdd = fg(t.Diff.Add)
	s.DiffDel = fg(t.Diff.Del)

	s.Rule = fg(t.Border)
	chip := func(c color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c).Padding(0, 1)
	}
	s.Chips = map[ChipRole]lipgloss.Style{
		ChipNeutral: chip(t.Border),
		ChipSuccess: chip(t.Status.Success),
		ChipWarn:    chip(t.Warning),
		ChipError:   chip(t.Status.Failed),
		ChipRunning: chip(t.Status.Running),
	}
	box := func(b lipgloss.Border, c color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Border(b).BorderForeground(c)
	}
	s.Boxes = map[BoxKind]lipgloss.Style{
		BoxNormal:   box(lipgloss.NormalBorder(), t.Border),
		BoxActive:   box(lipgloss.NormalBorder(), t.Accent),
		BoxDashed:   box(dashedBorder, t.Border),
		BoxWarning:  box(lipgloss.NormalBorder(), t.Warning),
		BoxSelected: box(lipgloss.ThickBorder(), t.Accent),
	}

	s.Markdown = markdownStyle(t)
	s.Input = textinput.Styles{
		Focused: textinput.StyleState{Text: s.Text, Placeholder: s.Muted, Suggestion: s.Muted, Prompt: s.Accent},
		Blurred: textinput.StyleState{Text: s.Muted, Placeholder: s.Muted, Suggestion: s.Muted, Prompt: s.Muted},
		Cursor:  textinput.CursorStyle{Color: t.Accent, Shape: tea.CursorBlock},
	}

	s.status = map[gitlab.PipelineStatus]lipgloss.Style{
		gitlab.StatusSuccess:            s.Success,
		gitlab.StatusFailed:             s.Error,
		gitlab.StatusRunning:            fg(t.Status.Running),
		gitlab.StatusPending:            fg(t.Status.Pending),
		gitlab.StatusPreparing:          fg(t.Status.Pending),
		gitlab.StatusWaitingForResource: fg(t.Status.Pending),
		gitlab.StatusCanceling:          canceled,
		gitlab.StatusManual:             fg(t.Status.Manual),
		gitlab.StatusScheduled:          fg(t.Status.Manual),
		gitlab.StatusCanceled:           canceled,
		gitlab.StatusCreated:            s.Muted,
		gitlab.StatusSkipped:            s.Muted,
	}
	return s
}

func fg(c color.Color) lipgloss.Style {
	if c == nil {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(c)
}

// StatusStyle returns the style for a pipeline or job status.
func (s *Styles) StatusStyle(st gitlab.PipelineStatus) lipgloss.Style {
	if style, ok := s.status[st]; ok {
		return style
	}
	return s.Muted
}

// Status renders "✓ success"-style text: glyph and status word, both colored.
// The status word is GitLab's own and is never translated.
func (s *Styles) Status(st gitlab.PipelineStatus) string {
	return s.StatusStyle(st).Render(Glyph(st) + " " + string(st))
}

// StatusGlyph renders only the colored glyph.
func (s *Styles) StatusGlyph(st gitlab.PipelineStatus) string {
	return s.StatusStyle(st).Render(Glyph(st))
}

// Glyph returns the status glyph (SPEC §4).
func Glyph(st gitlab.PipelineStatus) string {
	switch st {
	case gitlab.StatusSuccess:
		return "✓"
	case gitlab.StatusFailed:
		return "✗"
	case gitlab.StatusRunning, gitlab.StatusPending, gitlab.StatusPreparing, gitlab.StatusWaitingForResource:
		return "◐"
	case gitlab.StatusManual:
		return "►"
	case gitlab.StatusCanceled, gitlab.StatusCanceling:
		return "⊘"
	default: // created, skipped, scheduled
		return "○"
	}
}

// NewInput returns a single-line text input styled from the theme.
func NewInput(s *Styles) textinput.Model {
	in := textinput.New()
	in.SetStyles(s.Input)
	in.Prompt = ""
	return in
}

// NewTextArea returns a multi-line input styled from the theme: no line
// numbers, no prompt, no cursor-line highlight.
func NewTextArea(s *Styles) textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.MaxHeight = 0
	plain := lipgloss.NewStyle()
	ta.SetStyles(textarea.Styles{
		Focused: textarea.StyleState{Base: plain, Text: s.Text, Placeholder: s.Muted, EndOfBuffer: plain, CursorLine: plain},
		Blurred: textarea.StyleState{Base: plain, Text: s.Muted, Placeholder: s.Muted, EndOfBuffer: plain, CursorLine: plain},
		Cursor:  textarea.CursorStyle{Color: s.Input.Cursor.Color, Shape: tea.CursorBlock},
	})
	return ta
}
