package issueactions

import (
	"strings"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/i18n"
	"github.com/4aykovski/glview/internal/ui/app"
)

func TestPreviewRendersMarkdownOncePerContentAndWidth(t *testing.T) {
	env := app.NewEnv(i18n.EN)
	env.Now = func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	cache := &RenderCache{}
	is := gitlab.Issue{IID: 1318, Title: "Surge", Description: "Zones with **override**.", UserNotesCount: 1}
	d := PreviewData{Issue: is, Cache: cache, State: NotesReady, Hint: true,
		Notes: []gitlab.Note{{ID: 1, Author: gitlab.User{Username: "egor"}, Body: "I take it.", CreatedAt: env.Now()}}}
	first := Preview(env, d, 60, 20)
	if cache.Renders != 2 { // the description and the note
		t.Fatalf("renders = %d", cache.Renders)
	}
	again := Preview(env, d, 60, 20)
	if cache.Renders != 2 || strings.Join(first, "\n") != strings.Join(again, "\n") {
		t.Fatalf("a repeated Preview must come from the cache: %d renders", cache.Renders)
	}
	Preview(env, d, 50, 20)
	if cache.Renders != 4 {
		t.Fatalf("another width renders again: %d", cache.Renders)
	}
	d.Issue.Description = "Changed."
	Preview(env, d, 50, 20)
	if cache.Renders != 5 {
		t.Fatalf("another text renders again: %d", cache.Renders)
	}
	cache.Reset()
	Preview(env, d, 50, 20)
	if cache.Renders != 7 {
		t.Fatalf("a reset (theme) renders again: %d", cache.Renders)
	}
	d.Issue.IID = 1319
	Preview(env, d, 50, 20)
	if cache.Renders != 9 {
		t.Fatalf("another issue starts over: %d", cache.Renders)
	}
}
