## Context

Задача #28 (T36), change отдельная (после MVP одна change — одна задача, задачи идут параллельно). Поведение — specs/ этой change, полная картина — docs/SPEC.md §2.6 (`T`), §3.0 (шапка), §3.17, §5.2, §5.7, §11.2, §13.1 п. 21. SPEC уже поправлен, код — по нему.

Что уже есть и переиспользуется:
- Таб Issues проекта (`screens/project/issues.go`) — образец списка REST со страницами по 20, `X-Total`, подгрузкой за 5 строк до конца, курсором по ID, превью снизу или справа (от 140 колонок), поллингом 30 с, пустыми состояниями по фильтрам, `s`/`backspace`.
- `common/table` (колонки с приоритетами, курсор по ID, подгрузка), `common.Markdown`, `common.HTMLToMarkdown`.
- `app.Env.Search` — фабрика оверлея, которую ставит `cmd/gitlab-tui/wire.go`, чтобы `app` не импортировал экраны; так же делается фабрика экрана To-Do.
- `app.Model.handleKey` — глобальные `p`/`F`/`?`/`ctrl+l`; `InputCapturer` отключает их в полях ввода.
- `Env.HeaderRight` — правая часть шапки; `Env.Net.Offline()`, `Env.AuthLost`.
- `mrdetail.New`, `issuedetail` (экран issue) — цели `enter`.
- `app.NewConfirm`, `app.OpenURL`, `app.Copy`, `Env.Fail`, `common.Gen`/`Token`, `app.Push`/`PopMsg`.
- `rest`: `mapUser`, `mapError`, пагинация через `X-Next-Page`/`X-Total` (как у issues).

## Goals / Non-Goals

**Goals:** видеть свои задачи To-Do и их число, открывать цель, отмечать выполненными без браузера.

**Non-Goals:** уведомления (desktop, звук), возврат выполненной задачи в pending, фильтры по проекту/автору/причине, «добавить в To-Do» из MR/issue.

## Decisions

1. **Экран, а не оверлей и не таб** (SPEC §13.1 п. 21). `screens/todos.Screen` реализует `app.Screen`; на стек кладёт корень по `T`.
2. **`T` в корне.** `Env.Todos func() app.Screen` (ставит `wire.go`); `nil` — `T` скрыт в хелпе. Правило стека: если в стеке есть экран, помеченный интерфейсом `app.TodoScreen` (метод-маркер), — закрыть всё выше него и отправить ему `ActivatedMsg` (он обновляется сам, как при возврате); иначе `Push`. Работает только без оверлея и без `InputCapturer`, как `p`/`F`.
3. **Счётчик в шапке.** `Env.TodoCount int` (-1 — неизвестен), `Env.TodoKnown`, отрисовка в `HeaderRight`: `@user · To-Do 5 · ⟳ 3s` (Accent), при 0 и неизвестном — нет; `9999+` при отсутствии `X-Total`. Корень запрашивает `Todos().Pending` после успешного `GET /user` (там, где корень сейчас узнаёт пользователя) и тиком 5 мин (`todoTickMsg` со своим `Gen`); пропуск тика при `Net.Offline()` и `AuthLost`; ошибка — debug-лог. Если шапке не хватает ширины, `To-Do N` выпадает первым (посмотреть, как шапка ужимается сейчас, и добавить его последним по приоритету).
4. **Обновление счётчика экраном.** Экран шлёт `app.TodoCountMsg{N}`: после ответа списка с `state=pending` и без типа (`Page.Total`), после `x` (N−1) и `X` (0). Корень сохраняет в `Env`.
5. **Список.** `TodoFilter{State: "pending"|"done", Type: ""|"MergeRequest"|"Issue"}`, страница 20, курсор по `Todo.ID`. Колонки и приоритеты — SPEC §3.17. Подписи `Action` — `i18n` map/функция по `action_name`, неизвестный — как есть.
6. **`x` оптимистично.** Строка убирается сразу (курсор — на ту же позицию), запрос `MarkDone`; ошибка — строка возвращается на прежнее место, тост ошибки, счётчик восстанавливается. Перезагрузка списка после успеха не обязательна (тик 30 с догонит), но кэш списка сбрасывается.
7. **`enter`.** `TargetType == "MergeRequest"` и есть проект — `mrdetail.New(env, *Project, nil, MergeRequest{IID, Title, WebURL}, false)`; `"Issue"` — экран issue; иначе — `app.OpenURL(TargetURL)`. Проекта нет — браузер.
8. **`o`** — `<хост>/dashboard/todos` + `url.Values` (`state=done` и `type=…` только не по умолчанию), `O`/`y` — `TargetURL`.
9. **REST.** client-go `Todos.ListTodos` (`State`, `Type`, `ListOptions`), `MarkTodoAsDone`, `MarkAllTodosAsDone`; `Pending` — `ListTodos(state=pending, per_page=1)` и `resp.TotalItems` (0 при отсутствии заголовка → вернуть -1). Маппинг цели: `target.iid`, `target.title`, `target.state`; для `Commit` — `target.id`/`short_id` в `TargetRef`. Проверить на JSON-фикстуре, что `target` разной формы (MR, issue, commit) разбирается без ошибки.

## Risks / Trade-offs

- [Фоновый запрос не от вершины стека] → один дешёвый запрос раз в 5 мин, отключается офлайн и без авторизации; записан в SPEC как исключение (§13.1 п. 21).
- [Нельзя вернуть выполненную задачу] → `X` под confirm, `x` — по одной, в state done видно, что сделано.
- [Цель в чужом/удалённом проекте] → MR detail/issue получат 404 и покажут тост «не найдено» (SPEC §11.2).
- [Параллельная работа T35/T37 трогает `client.go`, фейк, i18n, `app`] → новые файлы (`rest/todos.go`, `fake/todos.go`, `i18n/*_t36.go`), в общих — только добавления.
