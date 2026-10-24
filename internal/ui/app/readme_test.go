package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var backticked = regexp.MustCompile("`([^`]+)`")

// keyAtoms returns the single keys named in backticks of text: "`j/k`" gives
// j and k, "`ctrl+n/ctrl+p`" gives ctrl+n and ctrl+p.
func keyAtoms(text string) map[string]bool {
	out := map[string]bool{}
	for _, m := range backticked.FindAllStringSubmatch(text, -1) {
		for _, part := range strings.FieldsFunc(m[1], func(r rune) bool { return r == '/' || r == ' ' }) {
			out[part] = true
		}
	}
	return out
}

// section returns the text from the heading start to the next heading with
// the prefix end.
func section(t *testing.T, doc, start, end string) string {
	t.Helper()
	_, rest, ok := strings.Cut(doc, start)
	if !ok {
		t.Fatalf("no %q", start)
	}
	body, _, _ := strings.Cut(rest, end)
	return body
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The hotkey table of the README names only keys that SPEC §3 knows, and the
// global keys of the help overlay (?) are all in it.
func TestREADMEHotkeysMatchSpecAndHelp(t *testing.T) {
	readme := section(t, readRepoFile(t, "README.md"), "\n## Хоткеи", "\n## ")
	spec := section(t, readRepoFile(t, "docs/SPEC.md"), "\n## 3. Экраны", "\n## 4. ")
	shown, known := keyAtoms(readme), keyAtoms(spec)
	if len(shown) < 40 {
		t.Fatalf("README names only %d keys: is the table gone?", len(shown))
	}
	for k := range shown {
		if !known[k] {
			t.Errorf("README names the key %q that SPEC §3 does not", k)
		}
	}

	_, root, _, _ := newProbeRoot(t)
	for _, b := range root.globalBindings() {
		for _, part := range strings.FieldsFunc(b.Help().Key, func(r rune) bool { return r == '/' || r == ' ' }) {
			// help writes arrows as ↑/↓, the README as ↓/↑: same atoms.
			if !shown[part] {
				t.Errorf("help (?) lists the global key %q, README does not", part)
			}
		}
	}
}
