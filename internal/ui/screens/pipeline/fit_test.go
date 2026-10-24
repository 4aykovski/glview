package pipeline

import "testing"

// TestFitsTerminal is the SPEC §12.3 invariant for the pipeline screen: the
// stages, the help, the play dialog and the variables fit 80×24 and
// 120×40 in en and ru.
func TestFitsTerminal(t *testing.T) {
	d, _, _ := open(t, fixture())
	d.RequireFits()
	for _, keys := range [][]string{{"l"}, {"l"}, {"l"}, {"j"}, {"h", "h", "h", "h", "h"}} {
		d.Key(keys...)
		d.RequireFits()
	}
	d.Key("l", "l", "P") // the manual job: the play dialog
	d.RequireFits()
	d.Key("tab", "enter") // to the variables table
	d.RequireFits()
	d.Key("esc", "esc", "?")
	d.RequireFits()
}
