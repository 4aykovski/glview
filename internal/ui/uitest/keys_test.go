package uitest

import "testing"

func TestKeyPressStrings(t *testing.T) {
	for _, k := range []string{"j", "J", "?", "enter", "esc", "ctrl+l", "ctrl+c", "space", "tab", "shift+tab", "1", "alt+enter"} {
		if got := KeyPress(k).String(); got != k {
			t.Errorf("KeyPress(%q).String() = %q", k, got)
		}
	}
}
