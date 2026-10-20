package mrdetail

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestGoldenOverview(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.RequireGoldenSizes("mrdetail-overview")
}

func TestGoldenDiscussions(t *testing.T) {
	d, _, _, _ := setup(t, baseMR())
	d.Key("tab")
	d.RequireGoldenSizes("mrdetail-discussions")
}

func TestGoldenMergeDialog(t *testing.T) {
	mr := baseMR()
	mr.MergeStatus = "mergeable"
	mr.Squash = true
	d, _, _, _ := setup(t, mr)
	d.Key("M")
	d.RequireGoldenSizes("mrdetail-merge")
}

func TestGoldenMerged(t *testing.T) {
	d, _, _, _ := setup(t, mergedMR())
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.RequireGolden("mrdetail-merged-120x40")
}

func TestGoldenDiscussionsRich(t *testing.T) {
	d, _, _, client := setup(t, baseMR())
	client.SetDiscussions(path, 479, richDiscussions(t))
	d.Key("r", "tab")
	d.RequireGoldenSizes("mrdetail-discussions-rich")
}
