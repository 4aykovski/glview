package project

import (
	"testing"
)

// TestEndToEndProjectMode walks the main path of SPEC §12.2: MRs → Pipelines
// → Pipeline → Job log and back, switching the language on every screen.
func TestEndToEndProjectMode(t *testing.T) {
	client := newFake(3)
	addPipelines(client)
	client.SetTrace(2, "building\n\x1b[31mFAIL: TestSurge\x1b[0m\n")
	repo, _ := tempRepo(t)
	d, root, _ := open(t, client, repo)

	// MRs
	d.RequireView(path, "1 MRs", "MR number 1", "state:opened", "j/k down", "open")
	d.Key("ctrl+l")
	d.RequireView("3 из 3", "можно мержить", "Заголовок", "вниз", "открыть", "фильтр", "обновить")
	d.RequireNoView("j/k down")
	d.Key("ctrl+l")

	// Pipelines
	d.Key("2")
	d.RequireView("#102", "feat/surge")
	d.Key("ctrl+l")
	d.RequireView("Статус", "Источник", "лог", "новый пайплайн", "повтор")
	d.Key("ctrl+l")

	// Pipeline #101 (failed, newest first: 102, 101, 100)
	d.Key("j", "enter")
	if root.Depth() != 2 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView(path+" › Pipelines › #101", "test:integration")
	d.Key("ctrl+l")
	d.RequireView("стейдж", "джоба", "запуск", "повтор")
	d.RequireNoView("h/l stage")
	d.Key("ctrl+l")

	// Job log of the failed job
	d.Key("l", "enter")
	if root.Depth() != 3 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView("#101 › test:integration", "FAIL: TestSurge", "○ paused")
	d.Key("ctrl+l")
	d.RequireView("○ пауза", "следить", "свернуть", "поиск")
	d.RequireNoView("○ paused")
	d.Key("ctrl+l")

	// esc esc: back on the Pipelines tab
	d.Key("esc", "esc")
	if root.Depth() != 1 {
		t.Fatalf("depth = %d", root.Depth())
	}
	d.RequireView("2 Pipelines", "#102", "ID", "Ref")
	d.RequireNoView("FAIL: TestSurge")
}
