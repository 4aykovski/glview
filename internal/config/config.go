// Package config loads and saves ~/.config/glview/config.yaml and
// resolves which GitLab host and token to use.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/4aykovski/glview/internal/xdg"
)

// Config mirrors config.yaml (SPEC §7).
type Config struct {
	Host        string      `yaml:"host"`
	Token       string      `yaml:"token"`
	HostAliases []string    `yaml:"host_aliases"`
	Language    string      `yaml:"language"`
	Theme       string      `yaml:"theme"`
	TLS         TLS         `yaml:"tls"`
	Workspace   Workspace   `yaml:"workspace"`
	Project     Project     `yaml:"project"`
	RunPipeline RunPipeline `yaml:"run_pipeline"`
	GraphQL     GraphQL     `yaml:"graphql"`
	Polling     Polling     `yaml:"polling"`
	Glab        Tool        `yaml:"glab"`
	Git         Git         `yaml:"git"`
	Links       []LinkRule  `yaml:"links"`
}

// LinkRule makes the matches of Pattern in an MR title or description links
// (SPEC §7). URL is a regexp.Expand template.
type LinkRule struct {
	Pattern string `yaml:"pattern"`
	URL     string `yaml:"url"`
}

type TLS struct {
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type Workspace struct {
	PerProject       int `yaml:"per_project"`
	MaxGroupProjects int `yaml:"max_group_projects"`
}

type Project struct {
	PageSize int `yaml:"page_size"`
}

// Values of RunPipeline.RefsFirst.
const (
	RefsBranchesFirst = "branches"
	RefsTagsFirst     = "tags"
)

// RunPipeline configures the Run pipeline form (SPEC §3.9).
type RunPipeline struct {
	// RefsFirst is the group shown first in ref suggestions: "branches" or "tags".
	RefsFirst string `yaml:"refs_first"`
	// PrefillLocalBranch puts the current local branch into the ref field
	// and first among the suggestions.
	PrefillLocalBranch bool `yaml:"prefill_local_branch"`
}

type GraphQL struct {
	BatchSize int `yaml:"batch_size"`
}

type Polling struct {
	Active    Duration `yaml:"active"`
	Idle      Duration `yaml:"idle"`
	Workspace Duration `yaml:"workspace"`
}

// Tool configures an optional external binary (glab).
type Tool struct {
	// Enabled is "auto" (use if found) or "false".
	Enabled string `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// Disabled reports whether the tool was explicitly turned off.
func (t Tool) Disabled() bool { return t.Enabled == "false" }

type Git struct {
	Tool         `yaml:",inline"`
	Parallel     int      `yaml:"parallel"`
	Timeout      Duration `yaml:"timeout"`
	FetchOnStart bool     `yaml:"fetch_on_start"`
}

// Duration accepts Go duration strings ("3s", "1m") in YAML.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: invalid duration %q", n.Line, n.Value)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// D returns the value as time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// Default returns the configuration used when keys are missing.
func Default() Config {
	return Config{
		Language:  "en",
		Theme:     "terminal",
		Workspace: Workspace{PerProject: 3, MaxGroupProjects: 100},
		Project:   Project{PageSize: 20},
		RunPipeline: RunPipeline{
			RefsFirst:          RefsBranchesFirst,
			PrefillLocalBranch: true,
		},
		GraphQL: GraphQL{BatchSize: 10},
		Polling: Polling{
			Active:    Duration(3 * time.Second),
			Idle:      Duration(30 * time.Second),
			Workspace: Duration(60 * time.Second),
		},
		Glab: Tool{Enabled: "auto"},
		Git: Git{
			Tool:     Tool{Enabled: "auto"},
			Parallel: 6,
			Timeout:  Duration(60 * time.Second),
		},
	}
}

// DefaultPath returns the config path under the XDG config directory.
func DefaultPath() string { return filepath.Join(xdg.ConfigDir(), "config.yaml") }

// Warning is a non-fatal problem found while loading the config.
type Warning struct {
	Kind WarningKind
	Text string
	// Key, Value and Default describe a WarnValue warning: the dotted key,
	// the rejected value and the default used instead.
	Key, Value, Default string
}

type WarningKind int

const (
	// WarnParse means the file is not valid YAML; defaults are used.
	WarnParse WarningKind = iota
	// WarnPermissions means the file holds a token but is readable by others.
	WarnPermissions
	// WarnValue means a key has an unknown enum value; the default is used.
	WarnValue
	// WarnLink means a links rule is invalid and was dropped; Key names the
	// offending key (links[0].pattern) and Text the reason.
	WarnLink
)

// Load reads the config at path. A missing file is not an error. Parse
// errors are reported as warnings and the defaults are returned, so the
// program can still start.
func Load(path string) (Config, []Warning, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil, nil
	}
	if err != nil {
		return cfg, nil, fmt.Errorf("read config: %w", err)
	}

	var warns []Warning
	parsed := Default()
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return cfg, []Warning{{Kind: WarnParse, Text: err.Error()}}, nil
	}
	warns = append(warns, parsed.normalize()...)

	if parsed.Token != "" {
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
			warns = append(warns, Warning{
				Kind: WarnPermissions,
				Text: fmt.Sprintf("%s contains a token but has permissions %o; run chmod 600", path, info.Mode().Perm()),
			})
		}
	}
	return parsed, warns, nil
}

// normalize replaces invalid values with defaults. Unknown values of
// enum keys are reported as warnings.
func (c *Config) normalize() []Warning {
	d := Default()
	var warns []Warning
	if c.Language != "en" && c.Language != "ru" {
		c.Language = d.Language
	}
	switch c.Theme {
	case "terminal", "terminal-faint":
	case "":
		c.Theme = d.Theme
	default:
		warns = append(warns, badValue("theme", c.Theme, d.Theme))
		c.Theme = d.Theme
	}
	if c.Workspace.PerProject <= 0 {
		c.Workspace.PerProject = d.Workspace.PerProject
	}
	if c.Workspace.MaxGroupProjects <= 0 {
		c.Workspace.MaxGroupProjects = d.Workspace.MaxGroupProjects
	}
	if c.Project.PageSize <= 0 {
		c.Project.PageSize = d.Project.PageSize
	}
	if c.GraphQL.BatchSize <= 0 {
		c.GraphQL.BatchSize = d.GraphQL.BatchSize
	}
	if c.Git.Parallel <= 0 {
		c.Git.Parallel = d.Git.Parallel
	}
	switch c.RunPipeline.RefsFirst {
	case RefsBranchesFirst, RefsTagsFirst:
	case "":
		c.RunPipeline.RefsFirst = d.RunPipeline.RefsFirst
	default:
		warns = append(warns, badValue("run_pipeline.refs_first", c.RunPipeline.RefsFirst, d.RunPipeline.RefsFirst))
		c.RunPipeline.RefsFirst = d.RunPipeline.RefsFirst
	}
	warns = append(warns, c.checkLinks()...)
	return warns
}

// checkLinks drops the invalid links rules, one warning per rule.
func (c *Config) checkLinks() []Warning {
	var warns []Warning
	kept := c.Links[:0:0]
	for i, r := range c.Links {
		key, reason := "", ""
		switch {
		case strings.TrimSpace(r.Pattern) == "":
			key, reason = "pattern", "empty"
		case strings.TrimSpace(r.URL) == "":
			key, reason = "url", "empty"
		case !strings.HasPrefix(r.URL, "http://") && !strings.HasPrefix(r.URL, "https://"):
			key, reason = "url", "must start with http:// or https://"
		default:
			if _, err := regexp.Compile(r.Pattern); err != nil {
				key, reason = "pattern", err.Error()
			}
		}
		if key == "" {
			kept = append(kept, r)
			continue
		}
		full := fmt.Sprintf("links[%d].%s", i, key)
		warns = append(warns, Warning{Kind: WarnLink, Key: full, Text: full + ": " + reason})
	}
	c.Links = kept
	return warns
}

func badValue(key, value, def string) Warning {
	return Warning{
		Kind: WarnValue, Key: key, Value: value, Default: def,
		Text: fmt.Sprintf("%s: unknown value %q, using %q", key, value, def),
	}
}
