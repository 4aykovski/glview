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
