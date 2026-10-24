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

// readmeConfig returns the YAML example of the README section "Конфиг".
func readmeConfig(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	_, after, ok := strings.Cut(s, "\n## Конфиг")
	if !ok {
		t.Fatal("README has no Конфиг section")
	}
	_, block, ok := strings.Cut(after, "```yaml\n")
	if !ok {
		t.Fatal("no YAML example in the Конфиг section")
	}
	block, _, ok = strings.Cut(block, "```")
	if !ok {
		t.Fatal("unterminated YAML example")
	}
	return []byte(block)
}

// The README example must parse: every key exists in Config (a typo or a
// renamed key fails), values are valid and equal to the defaults.
func TestREADMEConfigExample(t *testing.T) {
	src := readmeConfig(t)

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

// Every top-level key of Config appears in the README example.
func TestREADMEConfigListsEveryKey(t *testing.T) {
	var shown map[string]any
	if err := yaml.Unmarshal(readmeConfig(t), &shown); err != nil {
		t.Fatal(err)
	}
	typ := reflect.TypeFor[Config]()
	for i := range typ.NumField() {
		key, _, _ := strings.Cut(typ.Field(i).Tag.Get("yaml"), ",")
		if _, ok := shown[key]; !ok {
			t.Errorf("README example lacks the key %q", key)
		}
	}
}
