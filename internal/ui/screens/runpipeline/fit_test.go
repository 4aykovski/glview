package runpipeline

import "testing"

// TestFitsTerminal is the SPEC §12.3 invariant for the Run pipeline form:
// the ref hints, the variables and the tag confirmation fit 80×24 and
// 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	d, _, _ := open(t, newFake(), "", "")
	d.RequireFits()
	d.Key("m")
	d.RequireFits()
	d.Key("down", "down", "enter", "tab", "a")
	d.RequireFits()
	d.Key("enter", "tab", "enter")
	d.RequireFits()

	d, _, _ = open(t, newFake(), "main", "feat/surge")
	d.Key("tab", "tab", "enter")
	d.RequireFits()
}
