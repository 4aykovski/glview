package todos

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/4aykovski/glview/internal/i18n"
)

func TestGoldenTodos(t *testing.T) {
	o := open(t, todoFake())
	o.d.RequireGoldenSizes("todos")
}

func TestGoldenTodosRu(t *testing.T) {
	o := openLang(t, todoFake(), i18n.RU)
	o.d.RequireGoldenSizes("todos-ru")
}

func TestGoldenTodosMentionPreview(t *testing.T) {
	o := openLang(t, todoFake(), i18n.RU)
	o.d.Key("j")
	o.d.RequireGoldenSizes("todos-mention-ru")
}

func TestGoldenTodosPreviewRight(t *testing.T) {
	o := open(t, todoFake())
	o.d.Send(tea.WindowSizeMsg{Width: 150, Height: 40})
	o.d.RequireGolden("todos-150x40")
}
