package gitremote

import (
	"bufio"
	"io"
	"strings"
)

// gitConfig is a minimal reader for git's INI-like config format. It
// supports sections with subsections (`[remote "origin"]`), comments,
// quoted values and repeated keys. Includes and conditional includes are
// not followed.
type gitConfig map[string]map[string][]string

func parseGitConfig(r io.Reader) gitConfig {
	cfg := gitConfig{}
	section := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			end := strings.LastIndexByte(line, ']')
			if end < 0 {
				continue
			}
			section = sectionKey(line[1:end])
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !found {
			value = "true"
		}
		if cfg[section] == nil {
			cfg[section] = map[string][]string{}
		}
		cfg[section][key] = append(cfg[section][key], parseValue(value))
	}
	return cfg
}

// sectionKey normalizes `remote "origin"` → `remote.origin`. Section names
// are case-insensitive, subsection names are case-sensitive.
func sectionKey(raw string) string {
	raw = strings.TrimSpace(raw)
	name, sub, hasSub := strings.Cut(raw, " ")
	name = strings.ToLower(name)
	if !hasSub {
		// Legacy form [section.subsection].
		if n, s, ok := strings.Cut(name, "."); ok {
			return n + "." + s
		}
		return name
	}
	sub = strings.TrimSpace(sub)
	sub = strings.TrimPrefix(sub, `"`)
	sub = strings.TrimSuffix(sub, `"`)
	sub = strings.ReplaceAll(sub, `\"`, `"`)
	sub = strings.ReplaceAll(sub, `\\`, `\`)
	return name + "." + sub
}

func parseValue(v string) string {
	v = strings.TrimSpace(v)
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case c == '\\' && i+1 < len(v):
			i++
			switch v[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(v[i])
			}
		case (c == '#' || c == ';') && !inQuote:
			return strings.TrimSpace(b.String())
		default:
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// get returns the last value of section.key (git's "last one wins").
func (c gitConfig) get(section, key string) string {
	vals := c[section][strings.ToLower(key)]
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// remotes returns the names of configured remotes that have a URL.
func (c gitConfig) remotes() []string {
	var names []string
	for sec, kv := range c {
		if name, ok := strings.CutPrefix(sec, "remote."); ok && len(kv["url"]) > 0 {
			names = append(names, name)
		}
	}
	return names
}
