package pipeline

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/ui/screens/joblog"
)

// attemptsLine is the panel summary "#305 ✓ · #303 ✗ · #302 ✗" within w
// cells; the attempts that do not fit are counted as "+N".
func attemptsLine(st *common.Styles, job gitlab.Job, w int) string {
	jobs := joblog.AttemptsOf(job)
	var b strings.Builder
	used := 0
	for i, j := range jobs {
		sep := ""
		if i > 0 {
			sep = " · "
		}
		part := fmt.Sprintf("%s#%d ", sep, j.ID)
		partW := ansi.StringWidth(part + common.Glyph(j.Status))
		// Room for this attempt and, unless it is the last, the "+N" after it.
		need := partW
		if left := len(jobs) - i - 1; left > 0 {
			need += ansi.StringWidth(fmt.Sprintf(" · +%d", left))
		}
		if i > 0 && used+need > w {
			b.WriteString(st.Muted.Render(fmt.Sprintf(" · +%d", len(jobs)-i)))
			break
		}
		b.WriteString(st.Muted.Render(part) + st.StatusGlyph(j.Status))
		used += partW
	}
	return b.String()
}
