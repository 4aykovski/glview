package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetKeepsCommentsAndOrder(t *testing.T) {
	path := writeFile(t, `# my gitlab
host: https://gitlab.example.com   # work instance
language: en # ui language

workspace:
  per_project: 3 # rows per group
`, 0o600)

	if err := Set(path, map[string]string{"language": "ru"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"# my gitlab", "# work instance", "# rows per group", "language: ru"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Index(s, "host:") > strings.Index(s, "language:") {
		t.Fatalf("key order changed:\n%s", s)
	}
	cfg, _, _ := Load(path)
	if cfg.Language != "ru" || cfg.Workspace.PerProject != 3 {
		t.Fatalf("reload: %+v", cfg)
	}
}

func TestSetCreatesFileWith0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	if err := Set(path, map[string]string{"host": "https://gitlab.com", "token": "glpat-abcdefghijklmnop"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o", perm)
	}
	cfg, _, _ := Load(path)
	if cfg.Host != "https://gitlab.com" || cfg.Token != "glpat-abcdefghijklmnop" {
		t.Fatalf("reload: %+v", cfg)
	}
}

func TestSetTightensPermissions(t *testing.T) {
	path := writeFile(t, "language: en\n", 0o644)
	_ = os.Chmod(path, 0o644)
	if err := Set(path, map[string]string{"language": "ru"}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o", perm)
	}
}

func TestSetRefusesBrokenYAML(t *testing.T) {
	path := writeFile(t, "language: [ru\n", 0o600)
	if err := Set(path, map[string]string{"language": "en"}); err == nil {
		t.Fatal("expected error for broken YAML")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "language: [ru\n" {
		t.Fatalf("broken file must not be overwritten, got %q", data)
	}
}
