package newmr

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"

	"github.com/4aykovski/glview/internal/ui/common"
)

// fieldID is a stop of the focus ring, top to bottom.
type fieldID int

const (
	fSrc fieldID = iota
	fDst
	fTitle
	fDraft
	fTemplate
	fDesc
	fAssignees
	fReviewers
	fLabels
	fMilestone
	fRemove
	fSquash
	fButton
	fieldCount
)

// item is a suggestion or a chosen value: a branch, a person, a label, a
// milestone.
type item struct {
	id   int64 // user or milestone ID; 0 for names
	name string
	note string // muted text after the name (a person's full name)
	me   bool   // the "@me" entry
}

// same reports whether a and b are the same value.
func (a item) same(b item) bool {
	if a.id != 0 || b.id != 0 {
		return a.id == b.id
	}
	return a.name == b.name
}

// hintInput is a text field with suggestions under it. A chip field also
// holds the chosen values; a plain one (a branch) takes the suggestion as
// its text.
type hintInput struct {
	id    fieldID
	in    textinput.Model
	chip  bool
	max   int // chips only: the most values, 0 = any number
	chips []item

	hints []item
	idx   int
	hide  bool // a suggestion was just used: hidden until the next edit
	dirty bool // edited since the suggestions were asked

	gen common.Gen // the suggestions request in flight
}

func newHintInput(s *common.Styles, id fieldID, chip bool, max int) *hintInput {
	in := common.NewInput(s)
	return &hintInput{id: id, in: in, chip: chip, max: max, gen: common.NewGen()}
}

func (h *hintInput) text() string { return strings.TrimSpace(h.in.Value()) }

// full reports a chip field that takes no more values.
func (h *hintInput) full() bool { return h.chip && h.max > 0 && len(h.chips) >= h.max }

func (h *hintInput) has(it item) bool {
	return slices.ContainsFunc(h.chips, it.same)
}

func (h *hintInput) names() []string {
	out := make([]string, len(h.chips))
	for i, c := range h.chips {
		out[i] = c.name
	}
	return out
}

// humanize is the title GitLab's web form makes of a branch name: "-" and
// "_" become spaces, the first letter is a capital.
func humanize(branch string) string {
	s := strings.NewReplacer("-", " ", "_", " ").Replace(strings.TrimSpace(branch))
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return ""
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// hasDraftPrefix reports a title that already marks a draft: "Draft:",
// "[Draft]" or "(Draft)", in any case.
func hasDraftPrefix(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, p := range []string{"draft:", "[draft]", "(draft)"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// withDraft is the title that goes to GitLab.
func withDraft(title string, draft bool) string {
	title = strings.TrimSpace(title)
	if draft && !hasDraftPrefix(title) {
		return "Draft: " + title
	}
	return title
}
