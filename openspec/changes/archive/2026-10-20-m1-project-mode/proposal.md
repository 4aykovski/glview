## Why

После M0 режим проекта — заглушка: шапка, табы и факты о проекте. Веха M1 делает из неё рабочий инструмент: список MR и пайплайнов текущего проекта, детали MR с действиями, пайплайн с джобами, живой лог джобы и запуск пайплайна с переменными. Это закрывает основной сценарий «открыл в папке проекта и сделал всё без браузера» (SPEC §1, цели 1, 3, 4). Задачи T11–T18 — docs/TASKS.md, поведение — docs/SPEC.md §3.4–3.9, §5, §10.3.

## What Changes

- Общий компонент таблицы: колонки с приоритетами, выделение, подгрузка страниц при скролле, пустое состояние и строки загрузки/ошибки.
- Экран Project вместо заглушки M0: табы MRs и Pipelines рабочие, Issues и Tags — заглушки «в M3».
- Таб MRs (GraphQL, фрагмент `MRRow`): фильтры state/scope/reviewer/draft/labels/поиск в строке `/`, фильтр «я» `m`, превью, `o`, `y`.
- Экран MR detail: вкладки Overview (markdown через glamour), Discussions, Pipelines; чипы статуса; поллинг 3/30 с.
- Действия над MR: approve/unapprove `a`, диалог merge `M` (сейчас / auto-merge, squash, удалить ветку, `sha`), rebase `b` с отслеживанием, checkout `C` через glab.
- Таб Pipelines (GraphQL, фрагмент `PipelineRow`): фильтры ref/status/source/user, `m`, превью стейджей, адаптивный поллинг, retry/cancel пайплайна, `l` — лог проблемной джобы, `n` — новый пайплайн.
- Экран Pipeline: стейджи колонками, bridges и downstream, панель джобы, play с переменными, retry/cancel джобы.
- Пакет разбора трейса джобы и экран Job log: секции, follow, поиск, горизонтальный скролл, обрезка 50 000 строк.
- Форма Run pipeline: ref с автодополнением веток и тегов, таблица переменных, подсказки из `.gitlab-ci.yml`.
- Инфраструктура, которой не хватало M0: сервисы gitlab для MR/пайплайнов/джоб (GraphQL-списки + REST-мутации за одним `gitlab.Client`), кэш и glab в окружении экранов, открытие браузера и копирование в буфер (OSC52), стиль glamour из `Theme`, реакция на 401 посреди сессии.

## Capabilities

### New Capabilities
- `project-view`: экран проекта, общая таблица списков, таб MRs и его фильтры.
- `merge-requests`: детали MR и действия над MR (approve, merge, rebase, checkout через glab).
- `pipelines`: таб Pipelines, экран пайплайна, действия над пайплайнами и джобами, запуск пайплайна.
- `job-log`: просмотр лога джобы в реальном времени: секции, ANSI-цвета, поиск, follow.

### Modified Capabilities
- `gitlab-access`: выбор параметра auto-merge по версии инстанса; деградация при отсутствии опциональных GraphQL-полей.
- `app-shell`: поведение при 401 посреди сессии; открытие ссылок в браузере и копирование в буфер; адаптивный интервал поллинга и фокус терминала.

## Impact

- Новые пакеты: `internal/joblog`, `internal/browser`, `internal/gitlab/hybrid` (сборка REST + GraphQL в один `gitlab.Client`), `internal/ui/common/table`, `internal/ui/screens/{project,mrdetail,pipeline,joblog,runpipeline}`.
- Расширение интерфейсов `internal/gitlab` (MergeRequestService, PipelineService, JobService, TagService.List, ProjectService.Branches/CIConfigVariables), REST-адаптера, GraphQL-запросов и фейка.
- Новая зависимость: `charm.land/glamour/v2` (+ chroma транзитивно).
- Правки SPEC: §4 (поле `MergeError`), §5.2 (фильтр «я» в режиме проекта, опрос rebase через `Get`), §6.1 (пакеты `hybrid`, `browser`).
- Экран-заглушка `start.Project` удаляется, `cmd/gitlab-tui/wire.go` собирает новый экран.
