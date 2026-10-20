## Context

Репозиторий пустой, есть только docs/SPEC.md и docs/TASKS.md. Мотивация — proposal.md, поведение — specs/ этой change. Архитектура пакетов, интерфейсы и выбор библиотек уже зафиксированы в SPEC §5–§9. Здесь — только решения, которые нужны для реализации M0.

Окружение: Go 1.26, golangci-lint установлен, glab на машине разработки пока не установлен (проверку токена из glab на живом бинаре делаем, когда он появится; до этого — фейковый бинарь в тестах).

## Goals / Non-Goals

**Goals:**
- Каркас, на котором экран M1 пишется без доработки инфраструктуры: стек, Styles, i18n, gitlab-интерфейсы, фейк, teatest-обвязка.
- Каждая часть покрыта тестами без сети.

**Non-Goals:**
- Реальные экраны списков (MR, пайплайны) — M1.
- Методы сервисов gitlab сверх нужных для старта (`Users.Current`, `Instance`, `Projects.Get`). Доменные типы SPEC §4 объявлены целиком, а интерфейсы сервисов растут по мере экранов: иначе REST-клиент и фейк пришлось бы заполнять заглушками методов M1–M3. Целевая форма — SPEC §6.3.
- Стиль glamour из `Theme` — первый markdown появится в M1 (MR detail), там и делаем.
- `internal/gitexec` — M2.

## Decisions

### Путь модуля — `github.com/4aykovski/glview`
По remote репозитория. Бинарь — `gitlab-tui`.

### Структура корня UI: стек + отдельный слой оверлеев
`app.Model` хранит `[]Screen` и необязательный `overlay` (Help, Confirm, позже picker/search/формы). Оверлеи не кладутся в стек: они не меняют вершину, не вызывают Activated/Deactivated и не останавливают поллинг под собой. Отрисовка — композиция слоёв Lip Gloss v2 (`lipgloss.NewLayer` + canvas) поверх View вершины.
Альтернатива — оверлеи как экраны в стеке: проще, но тогда список под Help перестаёт обновляться и мигает шапка; отвергнуто.

### Сообщения стека
`PushMsg`, `PopMsg`, `ReplaceRootMsg`, `OpenOverlayMsg`, `CloseOverlayMsg` обрабатывает только корень. При смене вершины корень сам рассылает `DeactivatedMsg` старой и `ActivatedMsg` новой вершине. `WindowSizeMsg`, `LangChangedMsg`, `ThemeChangedMsg` рассылаются всем экранам стека; остальное — оверлею, если открыт, иначе вершине.

### Поколения и поллинг
В `ui/common` тип `Gen` (uint64 с методом `Next()`), каждая команда захватывает значение, msg несёт `Gen`. Поллинг: `PollTickMsg{ScreenID, PollGen}`; экран на `DeactivatedMsg` делает `pollGen.Next()`, тики со старым значением игнорируются, на `ActivatedMsg` планирует новый тик. Так «поллинг только у вершины» обеспечивается без таймеров в корне.

### Keymap и футер
`common.Binding` = `key.Binding` + `Priority` + `Hidden`. Футер: сортировка по приоритету (стабильная, сохраняет порядок объявления при равенстве), жадное добавление, пока `lipgloss.Width` влезает; `?` с максимальным приоритетом всегда первым кандидатом, но отрисовывается последним. Чистая функция `RenderFooter(bindings, width, styles) string` — легко тестируется на en/ru.

### Конфиг через `yaml.Node`
Чтение — в структуру; запись — правка конкретных ключей в дереве `yaml.Node` исходного файла, чтобы сохранить комментарии и порядок. Запись атомарная: временный файл в том же каталоге с правами 0600 → `rename`.

### glab: два вызова с таймаутом
`exec.CommandContext` с таймаутом 3 с, `Stdin = nil`, без TTY. Сначала `glab config get token --host <host>`, затем `glab auth status --hostname <host> --show-token --output json` с разбором JSON (имя поля держим в одной константе, фиксируем тестом с записанным выводом). Тесты — фейковый `glab` (shell-скрипт в `testdata`), путь через `glab.path`.

### REST и общий транспорт
Свой `http.RoundTripper`-цепочкой: семафор на 6, 429-ретрай с `Retry-After`, маскирование токена в debug-логе. Этот же `http.Client` передаётся в client-go (`gitlab.WithHTTPClient`) и в GraphQL-клиент — лимиты общие.
Маппинг ошибок — одна функция `mapError(resp, err)` в `gitlab/rest`, GraphQL-клиент использует её для HTTP-уровня и свою для `errors`.

### GraphQL-батчер
Обобщённый `Batcher[T]`: на вход вид запроса (`kind`), фрагмент, функция декодирования группы и список путей. Сборка: `query Q($p0: ID!, …) { p0: project(fullPath: $p0) { ...F } … } fragment F on Project {…}`. Ошибка сложности распознаётся регуляркой `complexity of (\d+).*max complexity of (\d+)`. Выученные размеры — `map[kind]int` под мьютексом в клиенте. Результаты — канал, закрывается после последней пачки.

### Кэш
`cache.Cache` — `map[string]entry` под мьютексом + `golang.org/x/sync/singleflight` для единственного фонового обновления. API: `Get(ctx, key, ttl, fetch) (value, stale bool, err)` и `Revalidated() <-chan string` (ключи, обновлённые в фоне — корень превращает их в msg для экранов). Инвалидация — по префиксу ключа (`mr:<project>:<iid>`, `mrs:<project>:`).

### i18n — Go-структуры
`Messages` — вложенные структуры со строками и `func(...) string` для подстановок и плюралов; `en.go`/`ru.go` — литералы. Тест полноты обходит структуру через reflection: пустые строки и nil-функции — ошибка с путём поля.

### Тема
`common.Theme` с полями `color.Color`; `TerminalTheme(isDark)` по таблице SPEC §9.2; `NewStyles(Theme) *Styles`. Экраны держат `*Styles`, смена темы — замена указателя по `ThemeChangedMsg`. Корень на старте отправляет `tea.RequestBackgroundColor`. Запрет цветов в `ui/screens` — тест, который грепает исходники пакета на `lipgloss.Color(` и `Foreground(`/`Background(`.

### Линтер
golangci-lint v2: `depguard` (в `internal/ui/**` запрещены `gitlab.com/gitlab-org/api/client-go`, `github.com/4aykovski/glview/internal/gitlab/rest`, `…/internal/gitlab/graphql`), плюс стандартный набор (`govet`, `staticcheck`, `errcheck`, `revive` в мягком режиме).

### Тесты UI
Основной инструмент — `uitest.Driver`: синхронно вызывает `Update`, выполняет возвращённые команды (команды-таймеры дольше 150 мс отбрасываются, тики тесты шлют сами) и отдаёт `View()` без ANSI. Причина: рендерер Bubble Tea v2 пишет в терминал только изменённые ячейки, поэтому искать текст в сыром выводе teatest ненадёжно. Сквозная проверка через настоящий `tea.Program` — `uitest.Program` поверх `github.com/charmbracelet/x/exp/teatest/v2` (smoke-тесты). Размер по умолчанию 120×40.

## Risks / Trade-offs

- [API Bubble Tea/Lip Gloss v2 отличается от привычного v1 (View, слои, клавиши)] → начинаем с T01 «hello world» на v2 и подсматриваем в исходники модулей через `go doc`, а не по памяти.
- [glab не установлен на машине разработки] → токен из glab проверяется только фейковым бинарём; проверка на живом glab остаётся пунктом задачи и закрывается при первой возможности.
- [teatest v2 может быть экспериментальным и менять API] → тесты UI пишем через тонкую свою обёртку, при поломке правится одна точка.

## Open Questions

- Точное имя JSON-поля токена в `glab auth status --output json` — выяснить на живом glab (не меняет дизайн: одна константа).
