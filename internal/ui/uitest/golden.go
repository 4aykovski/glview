package uitest

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func init() {
	// teatest's golden helper already registers -update; share it.
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "rewrite golden files")
	}
}

func updating() bool {
	f := flag.Lookup("update")
	return f != nil && f.Value.String() == "true"
}

// normalize trims trailing spaces of every line and trailing blank lines, so
// a golden file does not depend on padding.
func normalize(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// RequireGolden compares the plain-text view with testdata/golden/<name>.txt
// of the calling package; run the tests with -update to rewrite the file.
func (d *Driver) RequireGolden(name string) {
	d.t.Helper()
	got := normalize(d.View())
	path := filepath.Join("testdata", "golden", name+".txt")
	if updating() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			d.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			d.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		d.t.Fatalf("golden file %s is missing (run with -update): %v", path, err)
	}
	if string(want) != got {
		d.t.Fatalf("view differs from %s (run with -update to accept):\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

// RequireGoldenSizes checks the view at 120×40 and then at 80×24, with the
// golden names name+"-120x40" and name+"-80x24".
func (d *Driver) RequireGoldenSizes(name string) {
	d.t.Helper()
	d.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	d.RequireGolden(name + "-120x40")
	d.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	d.RequireGolden(name + "-80x24")
	d.Send(tea.WindowSizeMsg{Width: Width, Height: Height})
}
