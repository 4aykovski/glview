package project

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestMRsDOpensTheDiff(t *testing.T) {
	client := newFake(3)
	client.SetMRDiffs(path, 2, []gitlab.DiffFile{{OldPath: "internal/slots/alloc.go", NewPath: "internal/slots/alloc.go", Diff: "@@ -1 +1 @@\n-a\n+b\n"}})
	d, root, scr := open(t, client, nil)
	d.RequireView("MR number 2")
	d.Key("j", "d")
	if _, ok := root.Top().(*Project); ok {
		t.Fatal("d must open the MR screen")
	}
	d.RequireView("!2", "Diff 1", "▾ internal/slots/alloc.go", "+1 −1")
	calls := client.CallsOf("MergeRequests.Diffs")
	if len(calls) != 1 || calls[0][1] != int64(2) {
		t.Fatalf("Diffs calls = %v", calls)
	}
	d.Key("esc")
	if mr := scr.tabs[0].(*mrsTab).selected(); mr == nil || mr.IID != 2 {
		t.Fatalf("esc must return to the list with the cursor on !2, got %+v", mr)
	}
}

func TestMRsFooterShowsDiffKey(t *testing.T) {
	d, _, _ := open(t, newFake(2), nil)
	d.RequireView("d diff")
}
