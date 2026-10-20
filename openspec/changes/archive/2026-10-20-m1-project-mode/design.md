## Context

M0 закончен (архив `2026-10-19-m0-foundation`): есть стек экранов, оверлеи (Help, Confirm), футер, тосты, `common.Gen`/`PollAfter`, i18n, тема, `cache.Cache` (не подключён к UI), GraphQL-клиент с батчером, REST-адаптер только для `Users.Current`, `Projects.Get`, `/version`, фейк, `uitest.Driver`. Режим проекта — заглушка `screens/start.Project`. Мотивация — proposal.md, поведение — specs/ этой change, полная картина — docs/SPEC.md §3.4–3.9, §5, §6, §10.3.

Важные факты о текущем коде:
- Корень раздаёт все не-клавиатурные msg всем экранам стека и оверлею (`app.Model.broadcast`), клавиши — оверлею, иначе вершине. Оверлей закрывается, вернув `nil` из `Update`.
- `app.Env` хранит `Client`, `User`, `Host`, `Styles`, `Msg`; кэша, glab и настроек в нём нет.
- `gitlab.Client` сейчас: `Users()`, `Projects()`, `Instance()`. Доменные типы SPEC §4 объявлены полностью.
- `glamour` ещё не в `go.mod`.

## Goals / Non-Goals

**Goals:**
- Рабочий режим проекта: MRs и Pipelines, MR detail, Pipeline, Job log, Run pipeline, действия над MR/пайплайнами/джобами, checkout через glab.
- Компоненты, которые M2 переиспользует без переделки: таблица, строка фильтров, `MRRow`/`PipelineRow`, действия над MR в отдельном пакете.
- Всё проверяемо без сети: фейк + `uitest`, фикстуры GraphQL/REST.

**Non-Goals:**
- Табы Issues и Tags (M3): только пустое состояние; `TagService` в M1 содержит только `List` для автодополнения ref.
- Комментарии к MR (`c` в MR) — M3.
- `u`/`U` (fetch/pull) в режиме проекта — M2 вместе с `internal/gitexec`.
- Checkout для проектов, открытых через `-p`, — только если cwd-репозиторий совпал по пути; сопоставление с папками workspace — M2.
- Офлайн-режим с экспоненциальной задержкой (SPEC §11.1) — M3 (T25, аудит ошибок). В M1 сетевые ошибки поллинга дают тост и следующий тик по обычному интервалу.

## Decisions

### 1. Один `gitlab.Client` поверх двух транспортов — пакет `gitlab/hybrid`
Списки и детали MR, список пайплайнов, стейджи и `ciConfigVariables` идут через GraphQL (SPEC §13.1 п.5), остальное — REST. UI видит один `gitlab.Client`. Новый пакет `internal/gitlab/hybrid` собирает его из `*rest.Client` и `*graphql.Client`: сервисы, у которых методы живут в разных транспортах (`MergeRequests`, `Pipelines`, `Projects`), — маленькие структуры-делегаты.
- `graphql` получает «запросы проекта» (`ProjectMRs`, `MR`, `ProjectPipelines`, `PipelineStages`, `CIConfigVariables`) — функции поверх `Client.Do`, возвращающие доменные типы. Тексты запросов — константы рядом с типами ответа, каждая с JSON-фикстурой в `testdata/`.
- `hybrid.New(rest, gql, me string)`: `me` — username текущего пользователя, нужен для `@me`, фильтра «я» и `ApprovedByMe`. `wire.go` строит hybrid после `Users().Current`.
- Альтернатива — GraphQL-методы внутри `rest.Client`: смешивает транспорты, ломает идею «rest = адаптер client-go». Отвергнуто.

Интерфейсы `internal/gitlab` растут до формы SPEC §6.3 в части, нужной M1:
- `MergeRequestService`: `List`, `Get`, `Discussions`, `Pipelines`, `Approve`, `Unapprove`, `Merge`, `Rebase` (без `Comment`).
- `PipelineService`: `List`, `Get`, `Stages`, `Create`, `Retry`, `Cancel`.
- `JobService`: `Get`, `Trace`, `Retry`, `Cancel`, `Play`.
- `TagService`: только `List(ctx, p, search, cursor, limit)`.
- `ProjectService`: + `Branches(search)`, `CIConfigVariables(ref)`.
- `Client`: + `MergeRequests()`, `Pipelines()`, `Jobs()`, `Tags()`.

Новые поля доменных типов (правка SPEC §4): `MergeRequest.MergeError string`, `MergeRequest.Squash bool`, `MergeRequest.ForceRemoveSourceBranch bool` (умолчания диалога merge). `Pipeline.YAMLErrors`, `Bridge.DownstreamProject` уже есть.

### 2. Нормализация данных GraphQL
- Глобальные ID (`gid://gitlab/Ci::Pipeline/123`) → `int64` одной функцией `parseGID`.
- Enum-ы GraphQL в верхнем регистре (`RUNNING`, `NOT_APPROVED`) → нижний регистр, как в REST. UI видит одно написание (`detailed_merge_status` «как есть» = REST-форма).
- `Appr`: `approvedBy.nodes`, `approvalsRequired`, `approvalsLeft`; `ApprovedByMe` — есть ли `me` среди `approvedBy`.
- `Thr` (неразрешённые треды) = `resolvableDiscussionsCount − resolvedDiscussionsCount`, без загрузки самих тредов. Если инстанс не знает этих полей — они опциональные (решение 3), колонка показывает `—`.
- Длительность — секунды → `time.Duration`.
- Сверить имена полей и аргументов (`draft`, `labels`, `search`, `reviewerUsername`, `stages{nodes{jobs{nodes{status}}}}`) на реальном инстансе и в схеме gitlab.com; при расхождении — поправить запрос и фикстуру. Это делается в задаче, а не здесь.

### 3. Опциональные поля GraphQL
Запрос собирается из шаблона с маркерами опциональных полей (`diffStatsSummary`, при необходимости `resolvedDiscussionsCount`). На ошибку `Field 'x' doesn't exist on type` клиент помечает поле как отсутствующее (map под мьютексом в `graphql.Client`, на сессию), пишет в debug-лог и повторяет запрос без него. Альтернатива — отдельный запрос версии и таблица «поле → с какой версии»: хрупко на self-hosted. Отвергнуто.

### 4. Фильтр «я» в режиме проекта — три соединения
GraphQL не умеет OR по author/assignee/reviewer. Список MR при `Mine` — три соединения (`a: mergeRequests(authorUsername: $me…)`, `s: …assigneeUsername`, `r: …reviewerUsername`) с собственными курсорами, **каждое отдельным запросом, параллельно** (`errgroup`; ошибка любого — ошибка всей страницы). Один запрос с тремя соединениями по 20 строк `MRRow` на реальном инстансе дал «Query has complexity of 301, which exceeds max complexity of 250». Страница = объединение загруженного, дедуп по IID, сортировка по `updatedAt desc`. `NextCursor` кодирует три курсора (`a|s|r`, пустой — у кончившегося). `Total` = объединение загруженного, UI пишет `≥`. При подгрузке возможна перестановка на стыке страниц — допустимо. Это та же схема, что SPEC §5.2 описывает для workspace; правка SPEC §5.2 — строка «Project → MRs».

`scope:created_by_me` → `authorUsername: me`, `scope:assigned_to_me` → `assigneeUsername: me`, `reviewer:@me` → `reviewerUsername: me`. Конфликт (`scope:created_by_me` + `m`) — `m` добавляет алиасы, а `scope` остаётся ограничением каждого алиаса.

### 5. Поллинг и кэш в экранах
- `app.Env` получает `Cache *cache.Cache`, `Glab *glab.Glab` (nil — нет glab), `Settings` (`PageSize`, `PollActive`, `PollIdle` из конфига) и `Version` (лениво, через `Instance().Version`, кэш на сессию). `ui` может импортировать `internal/cache` и `internal/glab` — depguard их не запрещает.
- Хелпер `common.Fetch[T](ctx, env.Cache, key, ttl, force, fn)`: при `force` — сеть + `Cache.Set`, иначе `cache.Get` (SWR). Первый показ и возврат на таб — без `force`; `r` и тики поллинга — `force`.
- Корень читает `Cache.Revalidated()` (listen-команда) и рассылает `CacheRevalidatedMsg{Key}`; экран, которому принадлежит ключ, повторяет загрузку без `force` с текущим токеном (это попадание в кэш, без сети).
- Ключи: `mrs:<path>:<filterSig>:<cursor>`, `mr:<path>:<iid>`, `mrdisc:<path>:<iid>`, `pipes:<path>:<filterSig>:<cursor>`, `pipe:<path>:<id>`, `stages:<path>:<iid>`. Инвалидация после действий — `Cache.Invalidate("mr:<path>:<iid>")`, `Invalidate("mrs:<path>:")` и т.п.
- Интервал выбирает экран: `PollActive`, если среди показанных объектов есть активные (`PipelineStatus.Active()`) или идёт rebase, иначе `PollIdle`. Тик несёт poll-токен (`common.Gen`, отдельный от gen данных); `DeactivatedMsg` → `pollGen.Next()`; `ActivatedMsg` → немедленный тик, если данные старше интервала, иначе планирование.
- Фокус терминала: корень включает репорт фокуса (`tea.View.ReportFocus`/аналог в v2), хранит `env.Focused`; экран умножает интервал на 5 (кап 5 мин). На `tea.FocusMsg` корень рассылает `FocusChangedMsg`, экран с просроченным тиком обновляется сразу.

### 6. 401 посреди сессии
Корень перехватывает новый `app.AuthLostMsg` (экраны шлют его через хелпер `common.HandleErr`/`app.ErrorCmd`, если `errors.Is(err, gitlab.ErrUnauthorized)`), ставит `env.AuthLost = true`, показывает тост с подсказкой по источнику токена (`env.TokenSource`). Экраны не планируют тики, пока `AuthLost`. `r` при `AuthLost` — корень делает `Users().Current`; успех → `AuthRestoredMsg` всем, экраны перезагружаются. Шапка экранов рендерит `✗ auth` через общий `common.Header`-хелпер, принимающий флаг.

### 7. Таблица — `internal/ui/common/table`
Не `bubbles/table`: там нет приоритетов колонок, строк-футеров и ID-курсора.
- `Column{Title string; Min, Max int; Flex bool; Priority int; AlignRight bool}`; `Row{ID string; Cells []string; Muted bool}`; заголовки и тексты — уже переведённые строки, ячейки уже стилизованы (стили из `Styles`).
- Ширины: отбрасываем колонки по возрастанию `Priority`, пока сумма `Min` + разделители не влезет; остаток делим между `Flex`; обрезка через `ansi.Truncate(cell, w, "…")`.
- Состояние: `cursor`, `offset`, `height`; `SetRows` сохраняет курсор по `ID`; `Tail` — `None | Loading | Error` (строки `◐ …`/`✗ …`); `Empty` — текст пустого состояния; `NearEnd(5) bool` — для подгрузки.
- Клавиши: `j/k/↓/↑/g/G/pgup/pgdown`; возвращает, сдвинулся ли курсор.
- Выделение: `Styles.RowSelected` (reverse) + маркер `›`.

### 8. Строка фильтров — `internal/ui/common/filterline`
`textinput` + описание ключей `[]Key{Name, Values []string (nil = любое), Multi bool}`. `tab` дополняет ключ или значение по префиксу. `Parse(s) (map[string]string, free string, err)`; ошибка («неизвестный ключ `foo`») показывается под строкой и не применяется. Экран реализует `app.InputCapturer`, пока строка в фокусе. Сериализация текущего фильтра обратно в `key:value` — для показа в строке фильтров и как стартовое значение при `/`.

### 9. Структура экрана Project
`screens/project.Project` держит шапку, табы и массив `tab`-моделей (`mrsTab`, `pipelinesTab`, `placeholderTab`) с интерфейсом `Init/Update/View/KeyMap/SetSize/Activate/Deactivate`. У каждого таба свой `common.Gen` данных и poll-токен; поллит только активный таб активного экрана. Локальный репозиторий (`*gitremote.Repo`) хранится в экране; `RefreshHEAD()` вызывается на каждой загрузке (`r`, тик) и после `ExecFinishedMsg`.
`start.Project` удаляется; `wire.modeScreen` создаёт `project.New(env, path, repo)`. Экраны 404 (`start.NewNotOnInstance`) остаются.

### 10. Действия над MR — пакет `screens/mractions`
Approve, merge-диалог, rebase-трекер и checkout используются из списка MR и из MR detail. Пакет отдаёт `tea.Cmd`-конструкторы и msg (`ActionDoneMsg{Kind, Ref, IID, Err}`), а экраны показывают тосты и перезапрашивают. Мутации не используют gen экрана (SPEC §6.4).
- **Merge-диалог** — свой оверлей (`app.Overlay`), а не huh: нужен `enter` = «подтвердить» с любого поля, а huh использует `enter` для перехода между полями. Контролы: радио «сейчас / auto-merge» (`←/→`), чекбоксы squash и «удалить ветку» (`space`), `j/k` по полям, `enter` — merge, `esc` — отмена. Правка SPEC §3.6 («Поля (huh)» → «свои контролы оверлея»).
- **Auto-merge по версии** — в REST-адаптере: `Merge` спрашивает `Instance().Version` (кэш на сессию в `rest.Client`) и ставит `auto_merge` (≥ 17.11) или `merge_when_pipeline_succeeds`. Сравнение версий — маленькая функция `versionAtLeast("17.11.2-ee", 17, 11)` с тестом.
- **Rebase** — `PUT …/rebase`, затем опрос `MergeRequests().Get` (GraphQL, поля `rebaseInProgress`, `mergeError`) каждые 2 с до 2 мин. Опрос — poll-токен экрана MR detail; если rebase запущен из списка, трекер живёт в табе MRs. Отступление от SPEC §5.2 (там REST `GET …?include_rebase_in_progress`): один `Get` на оба случая; правка SPEC §5.2.
- **Checkout** — `internal/glab`: `Action`, `BuiltinActions()` (`mr.checkout`, `C`, `glab mr checkout {{.MR.IID}}`, `NeedsRepo`, `NeedsGlab`), `Action.Cmd(TemplateData, dir)` по SPEC §10.3. UI: проверка `Glab.LoggedIn(host)` один раз за сессию (`tea.Cmd`, таймаут 3 с), затем `tea.ExecProcess(cmd, …)` → `app.ExecFinishedMsg{Action, Err}`. Биндинг `C` — `Hidden`, если `env.Glab == nil` или нет `repo`.

### 11. Markdown
`common.Markdown(s *Styles, md string, width int) string` на `glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(width))`, где `cfg ansi.StyleConfig` собирается в `NewStyles` из `Theme` (заголовки — Accent+Bold, код — Muted, ссылки — Running, цитаты — Muted; SPEC §9.2); при `NO_COLOR` — `glamour` `notty`. Результат кэшируется в экране по `(width, lang, styles ptr)`. Цвета задаются только в `ui/common`, тест-греп экранов продолжает проходить.

### 12. Экран Pipeline
- Данные: `Pipelines().Get` (REST: `GET /pipelines/:id`, `GET /pipelines/:id/jobs?include_retried=true&per_page=100` все страницы, `GET /pipelines/:id/bridges`). Ретраи: из джоб с одинаковым именем показываем последнюю, `RetryCount` = число более старых. Правка SPEC §5.2 (`include_retried=true` вместо `false`).
- Заголовок коммита: REST пайплайн его не содержит; если экран открыт из списка — берём `CommitTitle` из `PipelineSummary`, иначе один запрос `GET /repository/commits/:sha` (кэш 10 мин: коммит неизменяем) внутри `rest.Pipelines.Get`.
- Downstream другого проекта: `Bridge.Downstream.Project` = `ProjectRef{ID}`; экран пайплайна принимает `ProjectRef` и при необходимости догружает путь через `Projects().Get` для крошек.
- Раскладка: колонка стейджа фиксированной ширины (24, обрезка `…`), горизонтальное смещение `xoff` такое, что активная колонка видна. Колонка downstream — последней.
- Последняя строка лога: при выборе running/failed джобы — `Jobs().Trace` с отменой предыдущего, разбор через `joblog` и последняя непустая `Plain`.

### 13. `internal/joblog`
Без зависимостей от UI.
- `Parser.Write(chunk []byte)` — инкрементальный; хранит хвост незавершённой строки и незавершённой escape-последовательности/UTF-8. `Parser.Lines() []Line`, `Line{Text string /*с SGR*/, Plain string, Section int /*индекс или -1*/, Header bool}`; `Sections() []Section{Name, Title, Parent int, Start, End time.Time, Collapsed bool, FirstLine, LastLine int}`; `Partial() *Line` — незавершённая последняя строка для показа.
- `\r`: служебные `section_*` маркеры распознаются до обработки `\r`; иначе от строки остаётся текст после последнего `\r` (`\r\n` — обычный перевод строки).
- Escape: SGR (`ESC [ … m`) сохраняется, остальные CSI и OSC вырезаются. Ширина — `ansi.StringWidth`.
- Обрезка: `MaxLines = 50000`; при превышении удаляются первые строки, `Dropped int` растёт, индексы секций сдвигаются.
- Тесты: секции вложенные, `collapsed=true`, незакрытые; `\r`; вырезание CSI/OSC; свойство «Write кусками по любым границам (в т.ч. посреди escape и UTF-8) == Write целиком» на фикстурах из `testdata/` (реальный трейс с секциями); поиск.
- Экран: `screens/joblog` держит `Parser`, `consumed int`, видимые строки (с учётом свёрнутых секций), курсор/offset/xoff, follow, состояние поиска. Поиск по `Plain`, подсветка — `Styles.LogMatch`/`LogMatchCurrent` поверх `Plain`-сегмента (на подсвеченной строке SGR исходника не сохраняем — проще и читаемо).

### 14. Run pipeline и форма переменных
- Общий компонент `common/vartable`: таблица `KEY | VALUE | type`, режимы «навигация» (`a`/`d`/`enter`/`j/k`) и «редактирование ячейки» (`textinput`; тип переключается `space`), `InputCapturer` во время редактирования. Используется в Play (T16) и Run pipeline (T18).
- Play — оверлей `pipeline.playForm` с `vartable`; `ctrl+s`/кнопка «Запустить» — подтверждение.
- Run pipeline — оверлей `screens/runpipeline`: блок Ref (`textinput` + выпадающий список; debounce 300 мс через `tea.Tick` с токеном ввода — устаревшие тики игнорируются; ветки `Projects().Branches`, теги `Tags().List`; локальная ветка первой), блок переменных (`vartable` + подсказки). `ciConfigVariables` запрашивается при смене ref (после debounce); `null` → повтор через 1, 2, 4 с, затем «подсказки недоступны». После `Pipelines().Create` — `CloseOverlay` + `Push(pipeline.New(...))`.

### 15. Браузер и буфер
`internal/browser.Open(url) error`: `open`/`xdg-open` через `exec.Command(...).Start()` с `Stdout/Stderr = nil`, не ждём. Это не git, правило «git только в gitexec» не задето. В UI — `tea.Cmd`, ошибка → тост с ссылкой в подсказке. Буфер — `tea.SetClipboard(text)` (OSC 52). Правка SPEC §6.1 (пакеты `browser`, `gitlab/hybrid`).

### 16. Тесты UI
`uitest.Driver` + фейк. Фейк расширяется до новых интерфейсов: in-memory MR, пайплайны, джобы, трейсы (можно «дописывать» трейс между вызовами), ветки, теги, ci-переменные; журнал вызовов с аргументами (для проверки `sha` в `Merge`, переменных в `Play`/`Create`). Тики поллинга в тестах доставляются явно (`Driver.Send(PollTickMsg)`), как в M0.

### 17. Подсказки в строке фильтров
`filterline` уже умеет `Suggest`, но список не рисуется. Модель получает выпадающий список под полем (до 8 строк, выбор `↑/↓`/`ctrl+p/n`, `tab`/`enter` — подставить) и источник подсказок на ключ:
- `Key{Name, Desc, Values}` — статичные значения; новое поле `Source func(prefix string) tea.Cmd` для динамических (пользователи, метки, ref). Ответ — `SuggestionsMsg{Token, Key, Items []Suggestion{Value, Note}}` с токеном ввода: устаревшие отбрасываются, как в Run pipeline (debounce 300 мс через `tea.Tick` с токеном).
- Подсказки ключей показывают описание (i18n) и текущее значение фильтра.
- Новые методы `ProjectService`: `Members(ctx, p, search) ([]User, error)` — REST `GET /projects/:id/members/all?query=&per_page=20`; `Labels(ctx, p, search) ([]string, error)` — REST `GET /projects/:id/labels?search=&per_page=20`. Ref — существующие `Branches` и `Tags().List`. Ответы кэшируются на 5 мин по `(project, kind, prefix)`.
- Альтернатива — отдельная панель фильтров (оверлей со всеми полями и переключателями): второй способ сделать то же самое, больше кода и хоткеев. Отвергнуто: строка `/` с подсказками плюс `s`/`backspace` закрывают и частую смену state, и редкие поля.

### 18. Быстрая смена фильтров
`s` — цикл значений одного главного ключа таба (`state` у MRs, `status` у Pipelines: any → running → failed → success → manual), `backspace` — сброс к значениям по умолчанию. Оба — обычные биндинги таба с подписью в футере и хелпе; новый gen и перезапрос, как при `/`+`enter`. Правка SPEC §3.1 (хоткеи) и §3.4.

### 19. Вид по макетам
Источник — `docs/design/*.dc.html` и `docs/design/README.md` (что обязательно, что иллюстративно).
- `ui/common` получает строительные блоки, экраны собирают из них: `Rule(width)` (горизонтальная линия `─` цвета Border), `VRule(height)`, `Chip(text, role)` (рамка цвета роли), `Box(content, w, h, kind)` с видами `Normal | Active (Accent) | Dashed` (своя `lipgloss.Border` с `╌ ╎`), `FilterBar(keys...)` (префикс `/` Accent, ключи Muted). Новые стили — в `Styles`, цвета только из `Theme`; тест-греп экранов продолжает проходить.
- Каркас (`app`): линия над футером и правая часть футера (тост или `StatusHint()` экрана — опциональный интерфейс `app.StatusProvider`).
- Счётчики табов: MRs — `Page.Total` текущего фильтра, Pipelines — `Total`, если известен; в MR detail — число тредов и пайплайнов из загруженных данных.
- **Golden-тесты** (SPEC §12.2): `uitest` получает `RequireGolden(t, name)` — сравнение plain-текста View с `testdata/golden/<name>.txt`, обновление флагом `-update`. Набор: MRs, MR detail (Overview), Pipelines, Pipeline, Job log, Run pipeline — каждый на 120×40 и 80×24, данные фейка фиксированные, `env.Now` зафиксирован. Цвета golden не ловит (профиль Ascii) — они проверяются тестами стилей и вручную.

### 20. Замечания после проверки на живом инстансе
- **Статус не открытого MR.** GitLab отдаёт `detailed_merge_status = not_open` для merged/closed/locked — это не проблема, которую надо подсвечивать. Общий хелпер `render.MRStatusChip(mr)` (текст, роль) решает, что показать: для `State != opened` — `✓ merged` (Success) / `○ closed` / `○ locked` (нейтральные), иначе — прежняя логика по `detailed_merge_status`. Используют чип MR detail и превью таба MRs; `StatusLabel` для состояний не вызывается.
- **Ширина стейджей.** `colW` перестаёт быть константой: `stageWidth(w, n)` = `w / n`, если `n × 25 ≤ w`, иначе 25; остаток деления добавляется последней колонке. `visibleCols`, `ensureVisible`, `boxInner` и рамки берут ширину отсюда. Пересчёт при `WindowSizeMsg` и при загрузке пайплайна (число колонок меняется с появлением downstream).
- **Ref пайплайна MR.** `render.PipelineRef(ref string) string`: `refs/merge-requests/<iid>/(head|merge|train)` → `!<iid>`, иначе как есть. Только отображение — фильтр `ref:` и запросы получают исходную строку.
- **Appr.** `render.Approvals`: `n/req`, пока `n < req`; `n ✓` (Success), когда `req > 0 && n >= req`; `n` при `req == 0`. Чип MR detail — через `ChipApprovals` с тем же правилом (en/ru).

### 21. Время слияния и закрытия MR
- `gitlab.MergeRequest` получает `MergedAt`, `ClosedAt time.Time` (нулевые, если нет) и `MergedBy *User`. Поля `mergedAt closedAt mergeUser { username name }` добавляются только во фрагмент `MRDetail` (запрос деталей), не в `MRRow`: списки и фильтр `m` уже упирались в лимит сложности 250, а в таблице эти поля не нужны. Поля есть на целевом инстансе (проверено запросом к рабочему инстансу). В списках `MergedAt`/`ClosedAt`/`MergedBy` остаются пустыми.
- Шапка MR detail выбирает подпись по `State`: merged + `MergedAt` → `MR.Merged(by, age)`, closed + `ClosedAt` → `MR.Closed(age)`, иначе прежний `MR.Opened(age)`. Новые строки i18n en/ru: `merged by d.orlov 5h ago` / `смержил d.orlov 5ч назад`, без автора — `merged 5h ago` / `смержен 5ч назад`; `closed 3d ago` / `закрыт 3д назад`.
- REST-адаптер `Get` MR не обслуживает (`errGraphQLOnly`), фейк заполняет поля для смерженных и закрытых MR.

### 22. Треды карточками
Макета вкладки Discussions нет; вид собран из блоков §19 по образцу блоков MR detail.
- Карточка — `common.Box` во всю ширину (`BoxNormal`, у текущего — `BoxActive`); цвет рамки неразрешённого треда — новый вид/параметр Box с ролью Warning (цвета только из `Theme`). Между карточками пустая строка. Внутри: заголовок заметки, тело (`s.markdown(body, inner)`), `common.Rule` между заметками; ответы с отступом 2.
- HTML → markdown — отдельная чистая функция (например, `render.HTMLToMarkdown` или пакет в `internal/ui/common`), на `golang.org/x/net/html` (уже в зависимостях как indirect — сделать прямой). Таблица: первая строка с `<th>` (или первая строка) — заголовок, `|` в ячейках экранируется, переносы внутри ячейки — пробел. Результат кэшируется вместе с рендером markdown (ключ — тело заметки и ширина).
- Модель вкладки: список карточек с их высотами и смещением начала; `J/K` меняют текущий индекс и `scroll` = начало карточки; `j/k` меняют `scroll`, текущий = первая карточка, пересекающая верх окна. Свёрнутость — `map[discussionID]bool` с явными переключениями пользователя поверх правила по умолчанию (свёрнут, если resolved или тело первой заметки > 20 строк после рендера).
- `space` на вкладке Discussions — сворачивание (на других вкладках остаётся листанием); `pgdown/ctrl+d` листают везде. Новые подписи футера/хелпа (`J/K threads`, `space fold`) — en/ru.
- Ссылка треда — `mr.WebURL + "#note_" + id` первой заметки; `o`/`y` на других вкладках — как раньше (MR).
- Текущий тред — отдельный вид `BoxSelected` (толстая рамка `lipgloss.ThickBorder`, цвет Accent), а не `BoxActive`: в тёмной теме Accent и Warning оба жёлтые, и по одному цвету текущий неразрешённый тред не отличался от остальных (найдено при ручной проверке). `BoxActive` у стейджей Pipeline и полей форм не меняется.

### 23. Перепроверка `unchecked`
- Проверено на рабочем инстансе: у многих открытых MR статус `UNCHECKED`; `GET …/merge_requests/:iid?with_merge_status_recheck=true` вернул `checking`, через 2 с REST и GraphQL отдают реальный статус.
- `MergeRequestService.RecheckMergeStatus(ctx, p, iid) (string, error)` — REST (client-go `GetMergeRequestsOptions` с перепроверкой; если поля в client-go нет — параметр запроса вручную), `hybrid` направляет его в REST, фейк переводит `unchecked` в `checking`, а следующий `Get` отдаёт итоговый статус, заданный тестом.
- MR detail: после загрузки MR, если `State == opened && MergeStatus == "unchecked"` и ключ `iid@sha` ещё не перепроверялся — `tea.Cmd` с `RecheckMergeStatus` (msg несёт Gen; ответ `checking` сразу обновляет чип), запоминается время запроса. `pollDelay` — active, пока статус в `{unchecked, checking, preparing}` и с запроса прошло меньше 60 с. Ошибка — только `debuglog`.
- Список MR не перепроверяет (это был бы запрос на каждую строку); сохранённый GitLab результат появится в списке при следующем обновлении.

## Risks / Trade-offs

- [Поля GraphQL отличаются между версиями self-hosted] → опциональные поля с повтором (решение 3), фикстуры с реального инстанса, ручная проверка на целевом инстансе в задачах T11/T12.
- [Фильтр «я» тремя алиасами даёт неточную пагинацию на стыке страниц] → помечаем счётчик `≥`; порядок внутри загруженного всегда корректен.
- [Полный `GET /trace` каждые 3 с на больших логах] → разбор только хвоста; при > 50 000 строк — обрезка; инкрементальный эндпоинт — после MVP (SPEC §5.6).
- [glamour v2 и стиль из Theme могут дать 256-цветные коды по умолчанию] → собираем `ansi.StyleConfig` полностью сами, тест: в выводе нет `38;5;`/`38;2;`.
- [ExecProcess и raw-режим на разных терминалах] → ручная проверка `C` на macOS Terminal/iTerm2; ненулевой код → тост.
- [Объём M1 велик для одной сессии] → задачи упорядочены так, что после каждой группы `make test lint` зелёный и есть рабочее приложение; коммит на задачу (T11…T18).

## Migration Plan

Нет данных для миграции. Конфиг не меняется (используются существующие `project.page_size`, `polling.*`). Откат — revert ветки `m1`.

## Open Questions

- Точный набор полей счётчика тредов на целевом инстансе (`resolvableDiscussionsCount`/`resolvedDiscussionsCount`) — проверяется в задаче T11; если нет ни того, ни другого, колонка Thr показывает `—`, это не меняет спеку.

## Решения, принятые при реализации

- `OpenURL` и `Copy` живут в `internal/ui/app` (а не в `common`): им нужен `app.ToastMsg`, а `app` импортирует `common`. Открытие браузера идёт через `Env.Browser` (подменяется в тестах).
- `CIConfigVariables` сообщает «ещё не готово» ошибкой `gitlab.ErrNotReady`, сигнатура интерфейса осталась как в SPEC.
- `rest.Client` отдаёт `errGraphQLOnly` для методов, которые обслуживает GraphQL (списки, `Get` и пайплайны MR, `Stages`); их подменяет `hybrid`.
- `rest.Pipelines.Get` заполняет `Pipeline.Stages` (стейджи в порядке пайплайна по ID джоб и bridges) — экран пайплайна берёт порядок колонок оттуда. Общая свёртка статусов — `gitlab.FoldStatus/NewStage`.
- `Discussions` в `DiscussionStats` равны `-1`, если инстанс не знает счётчики тредов; колонка Thr показывает `—`.
- Фильтр «я»: `Page.Total` — нижняя граница (наибольший `count` из трёх алиасов), страница может содержать до 3×`limit` записей (объединение загруженного).
- Пайплайны MR в деталях берутся из GraphQL (`MRPipelines`, тот же фрагмент `PipelineRow`, что в табе Pipelines): REST-эндпоинт не отдаёт пользователя, длительность и заголовок коммита. Колонки вкладки совпадают с табом Pipelines, ячейки Commit/User — общие хелперы `render.PipelineCommit/PipelineUser`.
- Таб Pipelines — заглушка до T15 (`placeholderTab`).
- Действия над MR (`mractions.Controller`) общие для списка и деталей: результат `ActionDoneMsg` обрабатывает только экран на вершине стека (иначе тосты дублировались бы, т.к. корень рассылает msg всем экранам стека); трекер rebase останавливается при уходе экрана с вершины.
- Запуск внешних программ и проверка `glab auth status` вынесены в `Env.Exec` и `Env.GlabLoggedIn` (подмена в тестах, проверка один раз за сессию).
- Последняя строка лога в панели джобы (экран Pipeline) берётся простым разбором хвоста трейса (`lastLine`), а не `internal/joblog`: нужна только одна строка без секций.
- Форма Run pipeline запрашивает `ciConfigVariables` при открытии и при выборе/вводе ref, совпавшего с найденной веткой или тегом (не на каждый набранный символ); подсказки выбираются цифрами `1–9` в блоке переменных.
- Поллинг лога джобы — фиксированные 3 с (замедляется ×5 без фокуса), только пока джоба активна; после финального статуса последнее обновление и остановка.
- `docs/TASKS.md` не содержит отметок выполнения для T11–T18 (чекбоксы только в ручном чек-листе M3), поэтому в 11.2 отметки не ставились.
- `filterline.Key.Source` — `func(ctx, prefix) ([]Suggestion, error)`, а не `func(prefix) tea.Cmd`, как в §17: модель сама оборачивает вызов в команду с токеном ввода и debounce 300 мс, экрану не нужно знать про токены. Ответы кэшируются на 5 мин на уровне таба (`sugg:<проект>:<вид>:<префикс>`).
- Подсказки значений сначала показывают статичные совпадения по префиксу (`@me`, `any`), затем ответ сервера; при пустом слове (после пробела) список только информационный: `enter` применяет фильтр, подставляет `tab`.
- Фейк `Members`/`Labels` ищет по вхождению подстроки без учёта регистра (как сервер), а порядок «префикс первым» обеспечивает `filterline`.
- `backspace` в футере имеет низкий приоритет и на ширине 120 не помещается; он всегда виден в хелпе (`?`).
- `StageSummary.Duration` — сумма длительностей джоб стейджа (REST — по джобам и bridges, GraphQL — `jobs{nodes{duration}}`), нужна блоку PIPELINE в MR detail.
- Чипы статуса MR в рамках занимают три строки: на терминале ниже 30 строк они рисуются одной строкой без рамок, подпись к `detailed_merge_status` (`needs approvals`) внутри чипа — только от ширины 110.
- Колонки стейджей в Pipeline — рамки шириной 25 (кадр + пробел), на 120 колонок помещается 4 + downstream при прокрутке; имя bridge уступает место подписи `· waiting` / `· no access`.
- Строка курсора в логе рисуется без SGR исходника (reverse не ломается вложенными цветами); номера строк всегда Muted, маркер `›` в отдельной колонке.
- Хоткеи `Run pipeline`: подсказка `ctrl+s` вынесена в кнопку «Запустить» (Accent), в строке клавиш осталось остальное.
- Линия над футером добавлена в корень (`app`): экранам отдаётся высота `height-2`; `StatusProvider` реализуют Project (по активному табу), MR detail, Pipeline и Job log.
- Ширина колонки стейджей (25) включает пробел-разделитель, рамка = ширина − 1; при прокрутке колонки остаются по 25 без растягивания, остаток делится только в растянутом режиме и достаётся последней колонке (`colWidth`). Следствие: на 120 колонках помещаются 4 колонки из 5 (раньше 5 при 24), тесты с downstream идут на ширине 150.
- `render.MRStatusChip(mr)` возвращает (текст, роль); `render.MergeChip(s, mr)` принимает MR целиком и красит по роли. Подпись к `detailed_merge_status` не показывается у не открытых MR. В `render.Approvals` подсветка «апрувнул я» (`ApprovedByMe`) сохранена, пока требование не набрано.
- Решение 21: если у merged/closed MR нулевые `MergedAt`/`ClosedAt` (поле не пришло), шапка падает на прежнее `opened …` по `CreatedAt`. Фейк заполняет `MergedAt`/`MergedBy` в `Merge` (предзаполненных merged/closed MR в нём нет — их задают тесты); `ClosedAt` фейк не ставит, так как действия закрытия в `MergeRequestService` нет.
- Решение 22: `HTMLToMarkdown` живёт в `internal/ui/common` (`htmlmd.go`). Фрагменты с тегами ищутся только вне fenced-блоков и markdown-спанов `` `…` `` (иначе `<div>` в коде ломался бы); без известных тегов текст возвращается как есть. Первая строка таблицы — всегда заголовок; ячейки под `rowspan` остаются пустыми. Тело `<details>` внутри ячейки обрезается до 40 символов (иначе строка отчёта бота растягивается на экран, а glamour режет заголовки колонок до `Верс…`); вне ячеек `<details>` выводится целиком.
- Решение 22: сворачивание и текущий тред хранятся по `Discussion.ID` (`curThread`, `fold`), поэтому переживают перезагрузку и поллинг; текущий сбрасывается на первый при смене таба. Свёрнутая карточка — рамка в одну строку с префиксом `▸ `; число ответов прижато вправо.
- Решение 22: в тёмной теме `Accent` и `Warning` — оба жёлтые, поэтому рамка текущего треда и рамка неразрешённого в ней различаются только положением; в светлой теме (Accent — синий, Warning — красный) различимы. Тесты проверяют цвета на светлой теме.
- Решение 22: подписи `J/K threads` и `space fold` в футере вытесняют `o browser`/`y copy` на ширине 120 на вкладке Discussions (они остаются в хелпе `?`).
- Перепроверка `unchecked` (§23): в client-go v1.46 у `GetMergeRequestsOptions` нет `WithMergeStatusRecheck` (оно есть только в опциях списков), поэтому `rest.RecheckMergeStatus` добавляет `with_merge_status_recheck=true` своим `RequestOptionFunc`; ради типа `retryablehttp.Request` `go-retryablehttp` стал прямой зависимостью в `go.mod`. Фейк: `SetRecheckResult` задаёт итоговый статус, `RecheckMergeStatus` переводит `unchecked` → `checking`, следующий `Get` отдаёт итог. Быстрый поллинг (3 с, до 60 с) продолжается и после ошибки перепроверки; ответ перепроверки применяется к чипу, только если статус всё ещё `unchecked` (опоздавший ответ не затирает свежие данные).
