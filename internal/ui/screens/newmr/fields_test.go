package newmr

import "testing"

func TestWithDraft(t *testing.T) {
	for _, tt := range []struct {
		title string
		draft bool
		want  string
	}{
		{"Fix slot race", true, "Draft: Fix slot race"},
		{"Fix slot race", false, "Fix slot race"},
		{"Draft: Fix slot race", true, "Draft: Fix slot race"},
		{"draft: fix", true, "draft: fix"},
		{"[Draft] fix", true, "[Draft] fix"},
		{"(DRAFT) fix", true, "(DRAFT) fix"},
		{"Drafting a plan", true, "Draft: Drafting a plan"},
		{"  padded  ", true, "Draft: padded"},
	} {
		if got := withDraft(tt.title, tt.draft); got != tt.want {
			t.Errorf("withDraft(%q, %v) = %q, want %q", tt.title, tt.draft, got, tt.want)
		}
	}
}

func TestHumanize(t *testing.T) {
	for in, want := range map[string]string{
		"feat/surge-pricing": "Feat/surge pricing",
		"fix_slot_race":      "Fix slot race",
		"":                   "",
		"éclair-fix":         "Éclair fix",
	} {
		if got := humanize(in); got != want {
			t.Errorf("humanize(%q) = %q, want %q", in, got, want)
		}
	}
}
