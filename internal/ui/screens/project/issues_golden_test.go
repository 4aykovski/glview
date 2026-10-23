package project

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestGoldenIssues(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.RequireGoldenSizes("issues")
}

func TestGoldenIssuesPreviewRight(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	o.d.RequireGolden("issues-150x40")
}

func TestGoldenIssuesRelatedPreview(t *testing.T) {
	o := openIssues(t, issuesFake(), nil)
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	o.d.Key("j")
	o.d.RequireGolden("issues-related-150x40")
}
