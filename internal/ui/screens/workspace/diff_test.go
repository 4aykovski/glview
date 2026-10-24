package workspace

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestGroupedMRsDOpensTheDiff(t *testing.T) {
	client := newFake()
	files := []gitlab.DiffFile{{OldPath: "internal/slots/alloc.go", NewPath: "internal/slots/alloc.go", Diff: "@@ -1 +1 @@\n-a\n+b\n"}}
	client.SetMRDiffs("platform/route-service", 482, files)
	client.SetMRDiffs("platform/route-service", 479, files)
	d, root, _ := openExpanded(t, client, testScope(), allPaths, nil)
	d.Key("2", "g", "d")
	if _, ok := root.Top().(*Screen); !ok {
		t.Fatal("d on a group header does nothing")
	}
	if n := client.CallCount("MergeRequests.Diffs"); n != 0 {
		t.Fatalf("Diffs calls on a header = %d", n)
	}

	d.Key("j", "d")
	if _, ok := root.Top().(*Screen); ok {
		t.Fatal("d on an MR row must open the MR screen")
	}
	d.RequireView("platform/route-service", "▾ internal/slots/alloc.go")
	calls := client.CallsOf("MergeRequests.Diffs")
	if len(calls) != 1 || calls[0][0] != "platform/route-service" {
		t.Fatalf("Diffs calls = %v", calls)
	}
}
