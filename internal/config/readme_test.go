package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// readmes are the README translations and the heading of their config section.
var readmes = []struct{ file, config string }{
	{"README.md", "\n## Config"},
	{"README.ru.md", "\n## Конфиг"},
}

// readmeConfig returns the YAML example of the config section of a README.
func readmeConfig(t *testing.T, file, heading string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", file))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	_, after, ok := strings.Cut(s, heading)
	if !ok {
		t.Fatalf("%s has no %q section", file, heading)
	}
	_, block, ok := strings.Cut(after, "```yaml\n")
	if !ok {
		t.Fatalf("no YAML example in the %q section of %s", heading, file)
	}
	block, _, ok = strings.Cut(block, "```")
	if !ok {
		t.Fatal("unterminated YAML example")
	}
	return []byte(block)
}

// The README examples must parse: every key exists in Config (a typo or a
// renamed key fails), values are valid and equal to the defaults.
func TestREADMEConfigExample(t *testing.T) {
	for _, r := range readmes {
		t.Run(r.file, func(t *testing.T) { readmeConfigExample(t, readmeConfig(t, r.file, r.config)) })
	}
}

func readmeConfigExample(t *testing.T, src []byte) {

	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	var strict Config
	if err := dec.Decode(&strict); err != nil {
		t.Fatalf("README config does not decode: %v", err)
	}

	cfg, warns, err := Load(writeFile(t, string(src), 0o600))
	if err != nil || len(warns) != 0 {
		t.Fatalf("Load: err = %v, warnings = %+v", err, warns)
	}
	if cfg.Host != "https://gitlab.example.com" || len(cfg.HostAliases) != 1 || len(cfg.Links) != 1 {
		t.Fatalf("example host, aliases and links: %+v", cfg)
	}

	// Everything else is shown with its default value.
	cfg.Host, cfg.HostAliases, cfg.Links = "", nil, nil
	if want := Default(); !reflect.DeepEqual(cfg, want) {
		t.Fatalf("README example differs from the defaults:\n got %+v\nwant %+v", cfg, want)
	}
}

// Every top-level key of Config appears in each README example.
func TestREADMEConfigListsEveryKey(t *testing.T) {
	for _, r := range readmes {
		t.Run(r.file, func(t *testing.T) {
			var shown map[string]any
			if err := yaml.Unmarshal(readmeConfig(t, r.file, r.config), &shown); err != nil {
				t.Fatal(err)
			}
			typ := reflect.TypeFor[Config]()
			for i := range typ.NumField() {
				key, _, _ := strings.Cut(typ.Field(i).Tag.Get("yaml"), ",")
				if _, ok := shown[key]; !ok {
					t.Errorf("README example lacks the key %q", key)
				}
			}
		})
	}
}
