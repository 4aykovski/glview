package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, warns, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if cfg.Language != "en" || cfg.Theme != "terminal" || cfg.Workspace.PerProject != 3 {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
}

func TestLoadBrokenYAMLWarnsWithLine(t *testing.T) {
	path := writeFile(t, "host: https://x\nlanguage: [ru\n", 0o600)
	cfg, warns, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || warns[0].Kind != WarnParse {
		t.Fatalf("want one parse warning, got %v", warns)
	}
	if !strings.Contains(warns[0].Text, "line") {
		t.Fatalf("warning should mention a line: %q", warns[0].Text)
	}
	if cfg.Host != "" || cfg.Language != "en" {
		t.Fatalf("broken config must fall back to defaults: %+v", cfg)
	}
}

func TestLoadValues(t *testing.T) {
	path := writeFile(t, `
host: https://gitlab.example.com
language: ru
host_aliases: [gitlab-work]
polling:
  active: 2s
workspace:
  per_project: 5
git:
  enabled: "false"
  parallel: 3
`, 0o600)
	cfg, warns, err := Load(path)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if cfg.Language != "ru" || cfg.Workspace.PerProject != 5 || cfg.HostAliases[0] != "gitlab-work" {
		t.Fatalf("values not loaded: %+v", cfg)
	}
	if cfg.Polling.Active.D() != 2*time.Second || cfg.Polling.Idle.D() != 30*time.Second {
		t.Fatalf("durations: %+v", cfg.Polling)
	}
	if !cfg.Git.Disabled() || cfg.Git.Parallel != 3 {
		t.Fatalf("git: %+v", cfg.Git)
	}
}

func TestLoadUnknownLanguageFallsBack(t *testing.T) {
	cfg, _, _ := Load(writeFile(t, "language: de\n", 0o600))
	if cfg.Language != "en" {
		t.Fatalf("language = %q", cfg.Language)
	}
}

func TestLoadWarnsOnWidePermissionsWithToken(t *testing.T) {
	path := writeFile(t, "token: glpat-xxxxxxxxxxxxxxxxxxxx\n", 0o644)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, warns, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || warns[0].Kind != WarnPermissions {
		t.Fatalf("want permissions warning, got %v", warns)
	}
}

func TestLoadNoPermissionWarningWithoutToken(t *testing.T) {
	path := writeFile(t, "language: ru\n", 0o644)
	_ = os.Chmod(path, 0o644)
	_, warns, _ := Load(path)
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings %v", warns)
	}
}

func TestLoadRunPipelineDefaults(t *testing.T) {
	cfg, warns, err := Load(writeFile(t, "language: ru\n", 0o600))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if cfg.RunPipeline.RefsFirst != RefsBranchesFirst || !cfg.RunPipeline.PrefillLocalBranch {
		t.Fatalf("run_pipeline defaults: %+v", cfg.RunPipeline)
	}
}

func TestLoadRunPipelineValues(t *testing.T) {
	cfg, warns, err := Load(writeFile(t, "run_pipeline:\n  refs_first: tags\n  prefill_local_branch: false\n", 0o600))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if cfg.RunPipeline.RefsFirst != RefsTagsFirst || cfg.RunPipeline.PrefillLocalBranch {
		t.Fatalf("run_pipeline: %+v", cfg.RunPipeline)
	}
}

func TestLoadUnknownRefsFirstWarnsAndFallsBack(t *testing.T) {
	cfg, warns, err := Load(writeFile(t, "run_pipeline:\n  refs_first: foo\n", 0o600))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunPipeline.RefsFirst != RefsBranchesFirst || !cfg.RunPipeline.PrefillLocalBranch {
		t.Fatalf("run_pipeline: %+v", cfg.RunPipeline)
	}
	if len(warns) != 1 || warns[0].Kind != WarnValue || warns[0].Key != "run_pipeline.refs_first" || warns[0].Value != "foo" || warns[0].Default != "branches" {
		t.Fatalf("want one value warning, got %+v", warns)
	}
}
