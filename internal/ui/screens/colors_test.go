// Package screens has no code of its own; this test guards the rule that
// screens never pick colors (CLAUDE.md): only styles from common.Styles.
package screens

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var forbidden = regexp.MustCompile(`lipgloss\.Color\(|lipgloss\.(Black|Red|Green|Yellow|Blue|Magenta|Cyan|White|BrightBlack)\b|\.Foreground\(|\.Background\(|\.BorderForeground\(|lipgloss\.AdaptiveColor`)

func TestScreensDoNotPickColors(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			line = strings.ReplaceAll(line, "context.Background()", "")
			if forbidden.MatchString(line) {
				t.Errorf("%s:%d picks a color: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
