package main

import (
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
	"github.com/4aykovski/glview/internal/ui/common"
)

func TestApplyConfigRunPipelineSettings(t *testing.T) {
	env := app.NewEnv(i18n.EN)
	cfg := config.Default()
	cfg.RunPipeline = config.RunPipeline{RefsFirst: config.RefsTagsFirst, PrefillLocalBranch: false}
	if toasts := applyConfig(env, cfg, nil, "config.yaml"); len(toasts) != 0 {
		t.Fatalf("toasts = %+v", toasts)
	}
	if !env.Settings.RefsTagsFirst || env.Settings.PrefillLocalBranch || env.Settings.PageSize != 20 {
		t.Fatalf("settings = %+v", env.Settings)
	}
}

func TestApplyConfigUnknownValueToast(t *testing.T) {
	env := app.NewEnv(i18n.EN)
	warns := []config.Warning{{Kind: config.WarnValue, Key: "run_pipeline.refs_first", Value: "foo", Default: "branches"}}
	toasts := applyConfig(env, config.Default(), warns, "config.yaml")
	if len(toasts) != 1 || toasts[0].Level != common.ToastWarn {
		t.Fatalf("toasts = %+v", toasts)
	}
	if txt := toasts[0].Text; !strings.Contains(txt, "run_pipeline.refs_first") || !strings.Contains(txt, `"foo"`) || !strings.Contains(txt, "branches") {
		t.Fatalf("toast text = %q", txt)
	}
	if env.Settings.RefsTagsFirst || !env.Settings.PrefillLocalBranch {
		t.Fatalf("defaults expected: %+v", env.Settings)
	}
}

func TestApplyConfigLinkRules(t *testing.T) {
	env := app.NewEnv(i18n.EN)
	cfg := config.Default()
	cfg.Links = []config.LinkRule{{Pattern: `[A-Z]+-\d+`, URL: "https://tracker.example.com/browse/$0"}}
	warns := []config.Warning{{Kind: config.WarnLink, Key: "links[0].pattern", Text: "links[0].pattern: empty"}}
	toasts := applyConfig(env, cfg, warns, "config.yaml")
	if len(env.Settings.LinkRules) != 1 || !env.Settings.LinkRules[0].Re.MatchString("ABC-123") {
		t.Fatalf("rules = %+v", env.Settings.LinkRules)
	}
	if len(toasts) != 1 || !strings.Contains(toasts[0].Text, "links[0].pattern") {
		t.Fatalf("toasts = %+v", toasts)
	}
}

func TestApplyConfigTheme(t *testing.T) {
	env := app.NewEnv(i18n.EN)
	cfg := config.Default()
	cfg.Theme = "terminal-faint"
	applyConfig(env, cfg, nil, "config.yaml")
	if !env.Styles.Theme.Faint || env.ThemeName != "terminal-faint" {
		t.Fatalf("theme = %+v, name = %q", env.Styles.Theme, env.ThemeName)
	}
}
