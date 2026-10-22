## Context

M0 и M1 закрыты (архив `2026-10-19-m0-foundation`, `2026-10-20-m1-project-mode`). Мотивация — proposal.md, поведение — specs/ этой change, полная картина — docs/SPEC.md §2.3, §2.7, §3.3, §3.11–3.12, §5.2–5.3, §5.7–5.8, §10.4.

Что уже есть и переиспользуется:
- `internal/gitlab/graphql/batch.go`: `BatchSpec`, `BuildQuery`, `Batch` (пачки параллельно ≤ 3, адаптация к лимиту сложности с запоминанием размера по `Kind`, деление пополам по таймауту), `decodeBatch` (`null` → `ErrNotFound`, частичные ошибки по алиасу), `SortGroups`. Типы `ProjectGroup[T]`, `BatchResult[T]` объявлены. Не хватает переменных на проект (нужны для `rootRef`).
- Фрагменты `MRRow` и `PipelineRow` (`graphql/mrs.go`), запрос стейджей, `MRFilter`, `PipelineFilter`.
- `gitremote.Scan`/`Classify` (сканер, `LocalProject`, `Kind`), `Repo.RefreshHEAD`.
- `cache.Cache` (TTL + SWR), `cache.Disk` (JSON-снимки с возрастом), `cache.State` с `Touch` (недавние) — ни `Disk`, ни `State` ещё нигде не подключены.
- `config`: `workspace.per_project`, `workspace.max_group_projects`, `git.*`, `polling.workspace` уже читаются.
- `app.Model` раздаёт все не-клавиатурные msg всем экранам стека (`broadcast`), клавиши — оверлею или вершине; есть `Push`, `Pop`, `ReplaceRoot`, оверлеи `Confirm`/`Help`.
- `ui/common/table` (колонки с приоритетами, курсор по ID, хвост «загружаю ещё»), `ui/common/filterline`, `ui/screens/mractions` (approve/merge/rebase/checkout), экран `project` с интерфейсом `tab`.
- Заглушки M0: `screens/start.Workspace`, `start.EmptyWorkspace`; `p`/`F` на них и на `NotOnInstance` дают тост «появится позже».

## Goals / Non-Goals

**Goals:**
- Workspace из папки и из группы с табами «Проекты», MRs, Pipelines; переход в Project и обратно без потери состояния.
- Минимум запросов: снимок с диска, объединённые фазы обзора, ленивые табы, точечный быстрый поллинг.
- `p`/`F` как оверлеи корня, доступные на любом экране.
- `internal/gitexec` с тестами на настоящем git; `u`/`U` в workspace и режиме проекта.

**Non-Goals:**
- Таб Tags workspace и всё про теги, кроме колонки «Тег» в «Проектах» — M3 (#23).
- Калибровка размера пачки через `queryComplexity` (SPEC §5.3 п.4, открытый вопрос §13.2 п.4) — остаётся адаптация по ошибке.
- Офлайн-режим с экспоненциальной задержкой — M3 (#25).
- Push, fetch по таймеру, ff default-ветки — после MVP (#35).
- Star/unstar в picker (`ctrl+s` из макета) — убран в SPEC §3.1.

## Decisions

### 1. `internal/workspace`: источники и Scope
- `ProjectSource` с двумя реализациями: `LocalDirSource{Dir, Instance}` (оборачивает `gitremote.Scan`) и `GroupSource{Group, Limit, Client}` (через `Projects().ListGroupProjects`). `Name()` — подпись для шапки и крошек (`~/src`, `group: platform`).
- `workspace.Project{Ref, Local *gitremote.LocalProject, Foreign bool}` — строка «Проектов». `Scope` хранит строки и производный список уникальных `ProjectRef` для батчей (дедупликация по `FullPath`, первая папка проекта — «основная» для групповых табов).
- Пакет не зависит от UI; `ui` импортирует `workspace` (depguard разрешает).
- Альтернатива — держать сканер прямо в экране Workspace. Отвергнуто: `GroupSource` и picker используют те же типы.

### 2. Сервисы `gitlab`
- Новый тип `ProjectOverview` (правка SPEC §4): `Project` (id, имя, `WebURL`, `DefaultBranch`, `LastActivityAt`, `Archived`), `OpenMRs`, `ReviewMRs int`, `DefaultPipeline *PipelineSummary`, `LastTag string`, `Phase int` (1 или 2 — что уже заполнено).
- `WorkspaceService`: `ProjectsOverview(ctx, ps []ProjectRef, o OverviewOptions)` (`OverviewOptions{RootRefs map[string]string, ForceTags bool}`: известные `rootRef` из снимка и признак `r` для тегов — правка SPEC §6.3), `MergeRequests(ctx, ps, f MRFilter, perProject)`, `Pipelines(ctx, ps, f PipelineFilter, perProject)` — все возвращают `<-chan BatchResult[T]`. `Tags` в интерфейс не добавляется до M3. `me` hybrid уже знает.
- `SearchService.Search(ctx, q) ([]Project, []Group, error)`.
- `ProjectService`: + `Starred(ctx) ([]Project, error)`, `ListGroupProjects(ctx, groupID, limit) ([]Project, int, error)` — второе значение — общее число (`X-Total`); сигнатура с `total` — правка SPEC §6.3.
- `MRFilter.SourceBranch string` (правка SPEC §6.3) — для MR локальной ветки в превью «Проектов»: `MergeRequests().List(p, {State: opened, SourceBranch: b}, "", 1)`.
- Фейк получает те же методы с журналом вызовов, сценарными задержками по проекту (для теста «пачки в обратном порядке») и `null`-проектами.

### 3. Обзор проектов: фазы и `rootRef`
- `BatchSpec` получает `PerProject func(i int, path string) (decl string, vars map[string]any)` — объявления и значения переменных конкретного алиаса (`$r3: String!`). Фрагменты GraphQL не принимают параметров, поэтому поле с переменной алиаса пишется в теле алиаса рядом со спредом: `p3: project(fullPath: $p3) { ...WsOverview dp: pipelines(ref: $r3, first: 1) { nodes { id iid status } } }`. Для этого в `BatchSpec` поле `Body func(i int) string` (по умолчанию — `...FragmentName`). Альтернатива — `pipelines` без `ref` и фильтрация на клиенте: последний пайплайн проекта часто не на default-ветке, отвергнуто.
- Kind пачки: `ws.overview` (фаза 1), `ws.overview.full` (1+2), `ws.overview.p2` (только пайплайны), `ws.mrs`, `ws.mrs.mine`, `ws.pipelines` — у каждого свой выученный размер.
- `rootRef` кэшируется в `cache.Cache` под ключом `rootref:<path>` с TTL 24 ч и в дисковом снимке. Пачки, где для всех проектов `rootRef` известен, идут как `ws.overview.full`; остальные — фаза 1, по ответу — фаза 2 только для проектов с полученным `rootRef` (у пустого репозитория `rootRef` нет — фаза 2 пропускается).
- Последний тег — REST `GET /repository/tags?order_by=updated&per_page=1` на проект, семафор 4, `cache.Cache` с TTL 10 мин (правка SPEC §5.8: строка «последний тег workspace — 10 мин»). TTL действует только для тиков поллинга: `r` идёт через `Fetch` с `force` (как везде, SPEC §5.7) и перезапрашивает теги всех проектов. Ответ тегов приходит отдельным msg и вклеивается в строку. Альтернатива — перезапрашивать с каждым тиком 60 с: N REST-запросов в минуту ради редко меняющегося поля, отвергнуто.
- Done-критерий #19 («одним-двумя GraphQL-запросами») при `batch_size: 10` и 10+ проектах означает: по одному запросу на пачку, без отдельной фазы 2. Проверяется по debug-логу на живом инстансе (ручной пункт).

### 4. Дисковый снимок workspace
- `cache.Disk` в `$XDG_CACHE_HOME/gitlab-tui/workspace/<sha256(host + "\n" + source)[:16]>.json`: список `ProjectOverview` без MR и пайплайнов, кроме статуса пайплайна default-ветки, плюс `rootRef` и время. Без токенов.
- При открытии Workspace снимок читается синхронно в конструкторе (файл маленький), строки рисуются сразу с `Stale`, затем стартует обзор. Снимок перезаписывается после каждого полного обзора (атомарно, `writeFileAtomic`).
- Снимок используется и в picker: число MR и активность для строк (SPEC §3.11).

### 5. Компонент групповой таблицы `ui/common/grouped`
- Над `table.Model` не надстраиваем: у групп свои строки (заголовок, «ещё N», «загрузка…», «нет доступа»), сворачивание и `J/K`. Новый пакет `ui/common/grouped`: `Model{groups []Group, collapsed map[string]bool, cursor rowID}`; `Group{Project ProjectRef, Header string, Rows []table.Row, More int, State loading|ready|empty|error}`. Колонки и их приоритеты считает та же функция, что в `table` (вынести `compute` в экспортируемый `table.Layout`), чтобы ширины и обрезка совпадали с режимом проекта.
- Курсор хранится как ID строки (`group/<path>`, `row/<path>/<id>`, `more/<path>`), после пересборки ищется по ID; если записи нет — остаётся в той же группе.
- Порядок групп — по `LastActivityAt` из обзора/снимка; группы без данных обзора — в конце по алфавиту. Пришедшая пачка только заполняет группы, порядок не пересчитывается до следующего полного обзора.

### 6. Экран Workspace
- Пакет `ui/screens/workspace`, интерфейс табов как у `project` (`Activate/Deactivate/Refresh/Count/Status/UpdatedAt`). Табы: `projectsTab`, `mrsTab`, `pipelinesTab`, `placeholder` (Tags).
- Батч-канал превращается в цепочку `tea.Cmd` (паттерн listen, SPEC §6.3): `wsBatchMsg{Tok, Tab, Result, next}`. Отмена — `context.CancelFunc` на таб, вызывается при смене фильтра, `r`, Deactivate и pop.
- Поллинг (правка SPEC §5.7): тик `polling.workspace`; в Pipelines, если есть running/pending в загруженных группах, — дополнительный тик `polling.active`, перезапрашивающий только эти проекты (батч из них). MRs — без быстрого тика: 3 с по всем проектам дорого, а статус пайплайна в MR не главное в обзоре.
- Строка фильтров: «Проекты» — `/` локальный текстовый фильтр; MRs — ключи `state` и свободный текст (`search`); Pipelines — `status`, `ref`. Переиспользуется `filterline` без подсказок с сервера.
- Действия над строками — те же функции, что в табах проекта: `mractions` для MR; действия пайплайна выносятся из `project/pipelines.go` в пакет `ui/screens/pipeactions` (retry, cancel, `l`, `n`), чтобы workspace и project их делили. `P` в списках нет (правка SPEC §3.3: убрать `P` из перечня действий workspace — в строке пайплайна нет выбранной джобы).
- Сводка группы MRs — `N open · M ждут тебя` (правка SPEC §3.3 и макета: «2 твоих» → «2 ждут тебя»): точное «моих» (author ∪ assignee ∪ reviewer) счётчиками GraphQL не получить без пересечений, а «ждут меня» уже есть в обзоре и совпадает с `·2` в «Проектах» (SPEC §13.1 п.7).
- Заглушки `start.Workspace` и `start.EmptyWorkspace` удаляются; пустое состояние — режим `projectsTab`.

### 7. Переход в Project
- `project.New(env, path, repo)` → `project.New(env, project.Options{Path, Repo, Tab, MRFilter, PipelineFilter, Crumb})`. `Crumb` — имя источника workspace; шапка `~/src › platform/route-service`.
- `f` на группе в табах берёт «основную» папку проекта (решение 1); на «Проектах» — папку строки.
- `esc` работает обычным `Pop`: Workspace лежит под проектом и сохраняет состояние сам.

### 8. Оверлеи `p` и `F`
- Один пакет `ui/screens/search` с двумя конструкторами (`NewPicker`, `NewGlobal`) и общим видом списка с секциями. Открывает корень по глобальным клавишам `p`/`F` (как `?`), если вершина не захватывает ввод (`InputCapturer`) и оверлей не открыт. Локальные `p`/`F`-заглушки в `start` удаляются.
- Источники — интерфейс `app.ProjectCatalog` в `Env`: `Current() *Entry` (проект cwd), `Workspace() []Entry` (из самого нижнего Workspace стека — корень регистрирует его при `Push/ReplaceRoot`), `Starred()`, `Recent()`, `LocalRepo(path) *gitremote.Repo`. Реализация в `ui/app` поверх `cache.Disk` и `cache.State`; сетевых вызовов не делает — это и проверяется тестом «журнал фейка пуст».
- Starred обновляется фоновым `tea.Cmd` из корня при старте, если снимок старше 1 ч; ошибка — debug-лог.
- Недавние: `Touch` в `project.Init` → запись `state.json` в фоне (атомарно).
- `F`: debounce через `tea.Tick(300ms)` с токеном ввода; тик со старым токеном игнорируется; при новом запросе предыдущий `ctx` отменяется. Порог — 3 символа (правка SPEC §3.12: «до 2 символов» → «меньше 3 символов»; GitLab не ищет частичные совпадения короче 3 символов).
- Fuzzy — `github.com/sahilm/fuzzy` по `FullPath`, совпавшие индексы подсвечиваются стилем Accent+Bold (`common.Styles`).
- Новый msg корня `OpenProjectMsg{Screen}`: если `stack[0]` — Workspace (интерфейс-маркер `app.WorkspaceRoot`), стек обрезается до него и проект кладётся сверху; иначе `replaceRoot`. Группа — обычный `ReplaceRoot` с новым Workspace (`GroupSource`). Закрытым экранам корень шлёт `DeactivatedMsg` и вызывает отмену (как при `pop`).

### 9. `internal/gitexec`
- `Runner` из SPEC §10.4. `exec.CommandContext` с `cmd.Stdin = nil` (→ `/dev/null`), `Env += GIT_TERMINAL_PROMPT=0, GIT_SSH_COMMAND` не трогаем, `SysProcAttr{Setsid: true}`, `cmd.Cancel` убивает группу процессов, `WaitDelay` 2 с. Unix-only (`//go:build unix`), на других ОС — заглушка «git недоступен» (проект под macOS/Linux).
- Pull: предпроверки из файлов (`Repo.Detached`, `branch.<b>.remote/merge` из `.git/config`, `MERGE_HEAD`, `rebase-merge/`, `rebase-apply/` в gitdir) → `git fetch --prune <remote>` → `git merge --ff-only @{u}`; `Moved` = behind до merge. Предпроверки доступны отдельно (`gitexec.Precheck(repo)`) — confirm `U` строит по ним список пропусков.
- ↑↓: `git rev-list --left-right --count HEAD...@{u}`; нет upstream — пусто без ошибки.
- Классификация stderr — таблица шаблонов из SPEC §10.4, причина — ключ i18n (`Reason` в `Result`), stderr — последние 20 строк.
- Для `gitremote.Repo` нужен `Remote` (имя remote) и `Upstream()` из конфига ветки — небольшое расширение пакета.

### 10. Git-операции в UI
- Состояние операции — `*localgit.Tracker` в `app.Env` (новый пакет `ui/localgit`, без зависимостей от экранов): идёт ли операция, `Op`, `done/total`, последние `Result` по `Dir`, ↑↓ по `Dir`. Запуск — `Tracker.Start(op, repos) tea.Cmd`; канал `Runner.Run` превращается в `GitProgressMsg`/`GitDoneMsg`. Корень уже раздаёт msg всем экранам, поэтому экран под стеком обновляется сам; шапку (`⟳ fetch 7/13`) рисует `common.Header` по `Env.Git`.
- Повторный `u`/`U` при `Tracker.Busy()` — тост «… уже идёт».
- В режиме проекта `u`/`U` — тот же Tracker с одним репозиторием. После pull — `Repo.RefreshHEAD` и пересчёт ↑↓.
- `git.fetch_on_start` — Workspace при первом `Activate` вызывает `Tracker.Start(OpFetch, …)`; ↑↓ при старте — `Tracker.AheadBehind(repos)` (≤ 8 параллельно).

### 11. Правки SPEC (до кода)
Одним коммитом в начале вехи (CLAUDE.md, «Процесс»):
- §3.3: сводка группы MRs `N open · M ждут тебя`; действия в групповых табах = действия списков режима проекта, без `P`; строка фильтров табов (решение 6); фильтры таба переносятся в Project при `f`; быстрый поллинг только проектов с running.
- §3.12: порог поиска 3 символа.
- §4: тип `ProjectOverview`.
- §5.2: строка «Workspace → Проекты» — объединение фаз через переменные `$rN`; MR локальной ветки в превью — `mergeRequests(sourceBranches:, state: opened, first: 1)`.
- §5.7: быстрый тик workspace Pipelines только по проектам с running/pending.
- §5.8: последний тег — 10 мин; путь снимка workspace.
- §6.1: пакеты `ui/common/grouped`, `ui/screens/pipeactions`, `ui/localgit`; `ui/screens` — `workspace`, `search`.
- §6.3: `ListGroupProjects` возвращает общее число, `MRFilter.SourceBranch`, `WorkspaceService` без `Tags` до M3, `project.Options`.

### 12. Правки по ревью PR #44

Найдены при ревью ветки `m2` перед мержем. Поведение, которое меняется, сначала закреплено в SPEC (правила батчинга, кэш на диске, git-действия, крайние случаи) и в дельта-спеках.

- **Fetch upstream-remote.** `gitexec` скачивает GitLab-remote проекта и, если `Repo.Upstream()` указывает на другой remote (не `.`), его тоже — одним вызовом `git fetch --prune <remote>` на каждый. Альтернатива «скачивать только upstream» отвергнута: `u` по SPEC обновляет remote проекта, на нём построены ссылки MR.
- **Частичные ошибки GraphQL не выбрасывают группу.** `phase1` (и фаза 2) берут `Items`, если они есть, даже при `g.Err`; ошибка с данными — debug-лог. Строка теряется, только если `Items` пуст (`project: null` или ошибка без данных). Отличать «данные + ошибка поля» в `decodeBatch` не нужно: решает наличие `Items`.
- **Starred и недавние на хост.** `starred/<sha256(host)[:16]>.json` по образцу снимка workspace. `State` → `{"hosts": {"<host>": {"recent": [...]}}}`; старое поле `recent` игнорируется. Ключ — только хост.
- **Повтор загрузки группы.** Ошибка `scopeLoadedMsg` запоминается на экране; `r` при не готовом scope с ошибкой перезапускает `Init`-загрузку списка (новое поколение), в шапке — состояние ошибки вместо вечного «загрузка».
- **Общее число неизвестно.** `ListGroupProjects` возвращает `total = -1`, если `X-Total` нет и есть следующая страница; `Scope.Total < 0` → «показаны N из многих» (новая строка i18n `ShownOfMany`).
- **Поколения ↑↓.** `AheadBehindMsg` несёт поколение папки в трекере; операция fetch/pull увеличивает его, устаревший подсчёт выбрасывается (правило `Gen` из CLAUDE.md).
- **`r` обходит кэш.** MR локальной ветки по `r` запрашивается с `force=true`, как остальные данные экрана.
- **Фильтр «Проектов»** сравнивает только имя папки (`filepath.Base(Dir)`) и путь проекта, как и сказано в SPEC §3.3; абсолютный путь не участвует.
- **Отчёты обзора.** `onOverview` пересобирает только вкладки, данные которых изменились (отчёт с тегами — только «Проекты»), и запрашивает MR локальной ветки, только если сменилась выбранная строка или её ветка/проект. Групповые MRs и Pipelines обзором не меняются — их не трогать.
- **Ссылки.** `shared.webURL` заменяется общим помощником из T27 (`projectURL`), одна реализация правила.

### 13. Свёрнутые группы, фильтр `P`, возврат из поиска, `ctrl+d/u` (#45–#48)

Поведение — SPEC §2.6, §3.1, §3.3, §3.11, §3.12, §5.7, §5.8 (коммит SPEC до кода).

- **Свёрнутость решает таб, а не таблица.** `grouped.Group` получает поле `Collapsed` и состояние `Expanding` (заголовок `▾` и строка `◐ загрузка…` под ним); `Empty` у раскрытой группы — заголовок и строка `нет открытых`. Своя карта свёрнутых и `space` из `grouped` убраны: набор раскрытых живёт в `groupBase.expanded` и пишется в `state.json`.
- **Владение проектом вместо одного поколения.** Запросов в табе может быть несколько одновременно (полная загрузка, раскрытие, перезапрос после действия, быстрый тик), поэтому у каждого свой токен из `seq`, а `owner[path]` помнит последний запрос проекта: пачка принимается только для проектов, чей последний запрос — она. Свёрнутая группа выбрасывается из `owner`, её поздний ответ игнорируется. Альтернатива «отменять всё при раскрытии» отвергнута: раскрытие во время полной загрузки обрывало бы её.
- **Раскрытие — пачкой.** `space`/`enter` кладут проект в `want` и при первом таком проекте ставят `tea.Tick(100 мс)`; тик запрашивает всё накопленное одним вызовом. Скрытый таб не запрашивает: при показе `fill()` догружает раскрытые группы без данных.
- **Заголовки свёрнутых — из обзора.** MRs — `OpenMRs`/`ReviewMRs`, Pipelines — `DefaultPipeline` (`✓ main #91790 · 2h`; для «2h» фаза 2 запрашивает `createdAt`). Тик таба 60 с и `r` перезапускают обзор с `OverviewOptions.SkipTags` (новое поле; теги — только на «Проектах»); если обзор уже идёт, второй не запускается. `onOverview` пересобирает групповые табы при изменении `OpenMRs`, `ReviewMRs`, ветки или пайплайна default-ветки.
- **Счётчики табов.** MRs — сумма `OpenMRs` из обзора, в режиме фильтра — число найденных; Pipelines — только в режиме фильтра (без фильтров число по свёрнутым группам не известно). Дополнение SPEC §3.3.
- **`state.json`.** `HostState.Workspaces[источник] = {at, expanded{mrs, pipelines}}`, лимит 50 на хост по `at`; `at` обновляется при открытии workspace, если запись есть. Сохранение — `Catalog.SetExpanded`/`OpenWorkspace`/`Touch` в фоне; у копий состояния номер версии, более старая копия не перезаписывает новую (фоновые записи могут закончиться в любом порядке). Повреждённый файл — пустое состояние целиком.
- **Режим фильтра (T30).** `narrowed()` у таба: MRs — текст, «я», state ≠ opened; Pipelines — status, ref, «я». В нём `targets()` — все проекты, прошедшие `P`, группы без записей скрыты, сворачивание — в `fcollapsed` (только в памяти); сброс фильтров возвращает к `expanded` и запрашивает только раскрытые. Расширение `P` в режиме фильтра догружает проекты, которых ещё не спрашивали (`fill()`), сужение не делает запросов.
- **Фильтр `P` (T30).** Своё поле `textinput` в `groupBase`, fuzzy `sahilm/fuzzy` по пути проекта, порядок групп — по активности, как без фильтра. Чип `проект: route ✕` и правая часть строки фильтров (`показаны 3 из 13 проектов` / `найдено в 4 проектах из 13`).
- **Стек и поиск (T31).** `OpenProjectMsg` обрезает стек до верхнего `WorkspaceRoot` (или до корня) и кладёт проект над ним; новый `OpenGroupMsg` обрезает до корня и кладёт Workspace группы. `Catalog` после каждого изменения стека получает верхний Workspace (`registerWorkspace`), секция WORKSPACE в `p` берёт его проекты. `ReplaceRoot` остаётся только у setup и экранов старта.
- **`ctrl+d`/`ctrl+u` в `p` и `F` (T32).** Оверлей перехватывает `ctrl+d`, `ctrl+u`, `pgdown`, `pgup` до `textinput`; шаг — половина высоты видимого списка (запоминается в `View`), курсор встаёт на ближайшую выбираемую строку в сторону движения. В хелпе — группа «Оверлеи p и F» (если поиск доступен).

### 14. Мульти-фильтр проектов `P` (#49)

Поведение — SPEC §3.1, §3.3 («Фильтр проектов `P`»), §5.8 (коммит SPEC до кода).

- **Оверлей, а не поле.** `P` открывает `app.Overlay` (`workspace.projectPicker`) над экраном, как `p`/`F`: клавиши идут в оверлей, табу не нужен `CapturesInput`. Список — `shared.order` (GitLab-проекты workspace по активности), отмеченные на момент открытия — сверху. Пока оверлей открыт, порядок не меняется: новая отметка не переносит строку, ввод только скрывает несовпавшие строки (без пересортировки по качеству совпадения) — строки не прыгают под курсором.
- **Применение — сообщением.** `enter` отдаёт `pickedMsg{tok, marked, text}`; токен — из `groupBase.pgen` (`Next()` при каждом открытии), поэтому сообщение применяет только тот таб и тот экран, который открыл оверлей (оно идёт широковещательно через корень и экран). `esc` закрывает оверлей без сообщения. Оверлей не делает запросов; в режиме фильтра расширение `P` догружает не запрошенные проекты через `afterP()`, как раньше.
- **Модель фильтра.** `groupBase.psel` (отмеченные) и `ptext` (текст) хранятся оба: повторный `P` открывает с ними. `applyP()`: есть отмеченные — `pmatch = psel`, иначе есть текст — fuzzy по пути, иначе `nil`. Чип: одна отметка — `проект: <путь> ✕`, несколько — `проекты: N ✕`, только текст — `проект: <текст> ✕`.
- **`state.json`.** `WorkspaceState.Selected` — карта таб → пути рядом с `Expanded`; `State.Selected/SetSelected` и `Catalog.Selected/SetSelected` с той же фоновой версионной записью. `SetSelected` с неизменным списком ничего не пишет. Отметки читаются в `newGroupBase`; когда известен scope (`Screen.setScope` → `onScope()`), пути вне workspace выбрасываются, без оставшихся фильтра нет. Текст не сохраняется. `backspace` (`resetP()`) очищает отметки и текст и пишет пустой список.
- **Общие куски списка.** Рамка оверлея, высота списка, ход `↓/↑`, полстраницы и подсветка fuzzy вынесены из `search` в `ui/common` (`ListModal`, `ListModalWidth/Rows`, `ListStart`, `ListMove`, `ListPage`, `Highlight`) и используются `p`, `F` и `P`; пакет `workspace` не может импортировать `search` (обратная зависимость).

## Risks / Trade-offs

- [Сложность обзора с `pipelines` на проект может превысить 250 при `batch_size: 10`] → батчер уже делит пачку и запоминает размер для `ws.overview.full`; проверка на живом инстансе в #19.
- [Аргумент `sourceBranches` или `searchNamespaces` отсутствует на старых инстансах] → ошибка схемы логируется; превью без MR локальной ветки, поиск — через REST (запасной путь из SPEC §5.2).
- [`Setsid` и убийство группы процессов не переносимы] → `//go:build unix`, цель MVP — macOS/Linux; на других ОС `u`/`U` скрыты как при отсутствии git.
- [Тесты gitexec с настоящим git медленные и зависят от версии git] → `t.Parallel`, локальные bare-репозитории через `file://`, «недоступный ssh» — `ssh://git@127.0.0.1:1/x.git` с `GIT_SSH_COMMAND="ssh -o ConnectTimeout=2 -o BatchMode=yes"` в тесте; при отсутствии git в окружении — `t.Skip`.
- [Дисковый снимок может показать удалённый проект] → снимок помечен `Stale`, строки обновляются первым же обзором; ошибка проекта перезаписывает снимок.
- [Рост `app.Env` (каталог проектов, Tracker)] → оба за узкими интерфейсами, экраны не знают реализаций; тесты подменяют их фейками.
- [Переход `f` переносит фильтры, а пользователь ждёт «чистый» проект] → `backspace` в проекте сбрасывает фильтры; поведение зафиксировано в SPEC.

## Migration Plan

Ветка `m2` от `main`, коммиты по задачам (`T19: … (#19)`), PR в `main` после всех задач. Конфиг не меняется: все ключи (`workspace.*`, `git.*`, `polling.workspace`) уже читаются. Дисковые файлы новые — откат = удалить `$XDG_CACHE_HOME/gitlab-tui/workspace/`.

## Open Questions

- Точное имя аргумента `sourceBranches` у `Project.mergeRequests` и поддержка `searchNamespaces` у `Query.projects` на целевом инстансе — проверяются в задачах по схеме; на спеки не влияют (есть запасные пути).
