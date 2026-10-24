package common

import (
	"strings"

	"github.com/4aykovski/glview/internal/gitlab"
)

// StyleDemo renders a sample of every style for manual checks on light and
// dark terminals and with NO_COLOR (glview --debug-styles).
func StyleDemo(s *Styles) string {
	var b strings.Builder
	line := func(parts ...string) { b.WriteString(strings.Join(parts, "  ") + "\n") }

	bg := "dark"
	if !s.Theme.IsDark {
		bg = "light"
	}
	line(s.AppName.Render("glview"), s.Header.Render("platform/route-service"), s.HeaderMuted.Render("· theme "+s.Theme.Name+", "+bg+" background"))
	line(s.TabActive.Render("1 MRs 6"), s.Tab.Render("2 Pipelines"), s.Tab.Render("3 Issues 12"), s.Tab.Render("4 Tags"))
	line(s.FilterKey.Render("state:")+s.FilterValue.Render("opened"), s.FilterKey.Render("reviewer:")+s.FilterValue.Render("@me"))
	line(s.ColumnHeader.Render("IID    Title                                   Pipeline"))
	line(s.Row.Render(" !482  Draft: add surge pricing to estimator  ") + s.Status(gitlab.StatusRunning))
	line(s.RowSelected.Render("›!479  Fix race in courier slot allocator     ✓ success"))
	line(s.RowMuted.Render(" dotfiles  github.com/egor/dotfiles · not GitLab"))
	line(s.GroupHeader.Render("▾ platform/route-service"), s.MoreRow.Render("↳ 3 more"))
	b.WriteString("\n")

	for _, st := range []gitlab.PipelineStatus{
		gitlab.StatusSuccess, gitlab.StatusFailed, gitlab.StatusRunning, gitlab.StatusPending,
		gitlab.StatusManual, gitlab.StatusScheduled, gitlab.StatusCanceled, gitlab.StatusSkipped, gitlab.StatusCreated,
	} {
		b.WriteString(s.Status(st) + "   ")
	}
	b.WriteString("\n\n")

	line(s.Success.Render("✓ approved !479"), s.Warning.Render("⚠ conflicts"), s.Error.Render("✗ Token rejected"), s.Accent.Render("accent"), s.Muted.Render("muted"))
	line(s.DiffAdd.Render("+ added line"), s.DiffDel.Render("- removed line"))
	line(s.LogLineNo.Render("51"), "--- "+s.LogMatchCurrent.Render("FAIL")+": TestSurge", s.LogMatch.Render("FAIL"))
	line(s.Button.Render("Cancel"), s.ButtonActive.Render("Yes"))
	b.WriteString(s.Preview.Render(s.PreviewTitle.Render("!479 Fix race in courier slot allocator")+"\n"+s.Muted.Render("preview box")) + "\n")
	line(s.Key.Render("j/k")+" "+s.KeyDesc.Render("move"), s.Key.Render("enter")+" "+s.KeyDesc.Render("open"), s.Key.Render("?")+" "+s.KeyDesc.Render("help"))
	return b.String()
}
