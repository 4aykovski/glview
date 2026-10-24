## Why

Задача #28 (T36) после MVP. Запросы ревью, упоминания и упавшие пайплайны своих MR приходят в To-Do GitLab, а увидеть их можно только в браузере. В MVP To-Do был в «Не-целях» (SPEC §1). Дорожная карта — #36.

## What Changes

- `gitlab`: `Todo`, `TodoFilter`, `TodoService` (`List`, `Pending`, `MarkDone`, `MarkAllDone`), `Client.Todos()` (#28).
- REST: `GET /todos` (страницы, `X-Total`), `POST /todos/:id/mark_as_done`, `POST /todos/mark_as_done`; `hybrid` делегирует; фейк (#28).
- Экран To-Do (`screens/todos`): список задач по инстансу, фильтры `s`/`t`/`backspace`, превью, `enter` к цели (MR detail, issue, браузер), `x`, `X` с confirm, `o`/`O`/`y`, поллинг 30 с (#28).
- Корень: глобальный `T` (правило стека — SPEC §2.6), счётчик `To-Do N` в шапке, запрос счётчика раз в 5 мин (#28).
- Строки — i18n en/ru, включая подписи причин (`action_name`) (#28).
- SPEC уже поправлен до кода: §1, §2.6, §3.0, §3.1, §3.14, §3.17, §4, §5.2, §5.7, §5.8, §6.1, §6.3, §11.2, §13.1 п. 21 (#28).

## Capabilities

### New Capabilities
- `todos`: экран To-Do, фильтры, действия, счётчик в шапке (#28).

### Modified Capabilities
- `app-shell`: глобальный `T` и счётчик To-Do в шапке (#28).

## Impact

- Код: `internal/gitlab` (types, client), `gitlab/rest` (`todos.go`), `gitlab/hybrid`, `gitlab/fake`, `internal/ui/screens/todos` (новый), `internal/ui/app` (корень, `Env`, шапка), `cmd/glview/wire.go`, `internal/i18n`.
- API: REST `/todos`. Новое фоновое обращение корня — один запрос раз в 5 мин.
- Зависимостей не добавляется.
