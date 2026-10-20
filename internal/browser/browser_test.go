package browser

import "testing"

func TestCommand(t *testing.T) {
	cases := []struct {
		goos, name string
		argc       int
	}{
		{"darwin", "open", 1}, {"linux", "xdg-open", 1}, {"freebsd", "xdg-open", 1}, {"windows", "rundll32", 2}, {"plan9", "", 0},
	}
	for _, c := range cases {
		name, args := command(c.goos, "https://x")
		if name != c.name || len(args) != c.argc {
			t.Errorf("%s: %q %v", c.goos, name, args)
		}
		if c.argc > 0 && args[len(args)-1] != "https://x" {
			t.Errorf("%s: url must be the last argument: %v", c.goos, args)
		}
	}
}
