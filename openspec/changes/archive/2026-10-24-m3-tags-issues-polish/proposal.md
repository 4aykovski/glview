## Why

M2 закрыт (архив `2026-10-22-m2-workspace`). Веха «M3 — теги, issues, формы, полировка» добивает MVP. Эта change — общая на веху: задачи вехи добавляются в неё по мере начала работы. Первая задача — #51 (T34). Дорожная карта — #36.

Сейчас заголовок MR — простой текст, кликабельны только markdown-ссылки в описании (glamour выводит их OSC 8). Ссылки на `#12`, `!45`, `@anna.k` и ключи трекера вида `ABC-123` в заголовке и описании не открываются. В tmux без `terminal-features hyperlinks` и в терминалах без OSC 8 не открываются вообще никакие ссылки.

Теги (#23, T23): табы Tags проекта и workspace — заглушки «появится позже». Нельзя посмотреть теги с пайплайнами, создать тег со следующей версией и удалить тег без браузера — а это частое действие при релизе (SPEC §1, цель 3).

Issues и комментарии (#24, T24): таб Issues — заглушка, комментарий к MR (`c`, SPEC §13.1 п.14) не сделан. Нельзя посмотреть issues проекта, прокомментировать issue или MR и закрыть issue без браузера (SPEC §1, цель 3).

Задачи вехи: #51 (T34), #23 (T23), #24 (T24) — сделаны (остались ручные проверки на живом инстансе); #25 (T25) — сделана (остался ручной чек-лист на живых инстансах).

Полировка (#25, T25): в 80×24 часть экранов не проверена снимками; ошибки 401/403/429 и офлайн (SPEC §11.1) не сверены с кодом целиком; Muted = ANSI 8 сливается с фоном в Solarized Dark (SPEC §13.2, вопрос 1); README описывает только M1.

## What Changes

- Пакет `internal/mrlinks`: `<a href>` из HTML GitLab, правила `links` из конфига, голые `http(s)://`, общий список без дублей в порядке текста, наложение OSC 8 на уже отрисованный текст без изменения видимого текста (#51).
- `gitlab.Link`, `MergeRequest.TitleLinks/DescriptionLinks`; GraphQL `MRDetail` запрашивает `titleHtml descriptionHtml` (опциональные поля `#@opt`), `MRRow` не меняется (#51).
- Конфиг: раздел `links: [{pattern, url}]`, проверка правил с тостом-предупреждением (#51).
- MR detail: title и описание на Overview со ссылками OSC 8; таб MRs (проект и workspace): Title и превью — OSC 8 по правилам и голым URL (#51).
- Хоткей `L` в MR detail и в табе MRs: оверлей «текст — URL», `enter` открыть, `y` копировать, `esc` закрыть; в табе MRs детали MR догружаются по `L`; нет ссылок — тост (#51).
- Строки — i18n en/ru (#51).
- Правки SPEC до кода: §3.1, §3.4, §3.5 («Ссылки MR»), §4, §5.2, §6.1, §7, §13.1 (#51).
- `gitlab.TagService` расширяется: `List` по `version desc` с релизами, `Pipelines` (пайплайны тегов), `Create`, `Delete`, `Compare` → `Comparison`; `MatchTagPipelines`; `WorkspaceService.Tags` — REST на проект, ≤ 4 параллельно (#23).
- Пакет `internal/semver`: разбор, сравнение, предложение patch/minor/major/release (#23).
- Таб Tags проекта: список страницами, поиск `/`, пайплайны тегов, превью с compare и «n предложит», `enter`/`n`/`D`/`y`/`o` (#23).
- Форма New tag (`screens/newtag`): варианты версии, ref с подсказками веток, сообщение, «В теге окажутся», кнопка в фокусе → confirm → создание (#23).
- `screens/tagactions`: удаление с confirm, тосты, инвалидация кэша, общее превью (#23).
- Групповой таб Tags workspace на `groupBase`: свёрнут по умолчанию, `expanded.tags`/`selected.tags` в `state.json`, поиск — режим фильтра, `f` → таб Tags проекта (#23).
- Строки — i18n en/ru; правки SPEC до кода: §3.1, §3.3, §3.4, §3.10, §4, §5.2, §5.8, §6.1, §6.3, §11.2 (#23).
- `gitlab.IssueService` (REST): список с фильтрами, `Get`, последние заметки, discussions, связанные MR, комментарий, close/reopen; `MergeRequestService.Comment`, `ProjectService.Milestones`; `Issue.MergeRequestsCount`, `RelatedMR` (#24).
- Таб Issues проекта: фильтры state/assignee/label/milestone, `s`/`m`/`backspace`, связанный MR в Title фоном, превью справа (≥ 140 колонок) или снизу с описанием и последними комментариями, `o`/`O`/`y` (#24).
- Экран issue (`screens/issuedetail`): описание и треды карточками; карточки тредов выносятся из `mrdetail` в общий пакет `screens/threads` без изменения вида MR detail (#24).
- Форма комментария (`screens/comment`): textarea, `tab` → кнопка `enter Отправить`, `alt+enter`, черновик в памяти сессии; `c` в Issues, на экране issue, в табах MRs и MR detail (#24).
- `x` close (confirm) / reopen (#24).
- Строки — i18n en/ru; правки SPEC до кода: §2.6, §3.1, §3.4, §3.5, §3.15, §3.16, §4, §5.2, §6.1, §6.3 (#24).
- Проверка 80×24 и ширины < 80 на всех экранах и оверлеях, en и ru; недостающие golden 80×24 (#25).
- Аудит ошибок по SPEC §11.1: маппинг REST/GraphQL, тосты и подсказки, 401 посреди сессии, офлайн с backoff и «соединение восстановлено» — тесты и исправления расхождений (#25).
- Тема `terminal-faint`: Muted через атрибут Faint вместо ANSI 8 (Solarized Dark); `theme` в конфиге получает второе значение (#25).
- README: workspace, поиск, теги, issues, комментарии, `links`, темы, хоткеи M2–M3 (#25).
- Правки SPEC до кода: §7, §9, §12.3, §13.2 (#25).

## Capabilities

### New Capabilities
- `issues`: таб Issues проекта, фильтры, превью, экран issue, close/reopen, форма комментария (#24).

### Modified Capabilities
- `merge-requests`: ссылки MR — источники, OSC 8 в title и описании, оверлей `L` в MR detail; комментарий `c` к MR (#24).
- `project-view`: OSC 8 в title списка MR, `L` в табе MRs; таб Tags, предложение версии, форма New tag, удаление тега (#23); заглушек табов больше нет (#24).
- `workspace`: то же в групповом табе MRs; групповой таб Tags, свёрнутые группы и `P` для Tags, `f` → Tags (#23); `c` в групповом табе MRs (#24).
- `configuration`: правила `links`; значение `theme: terminal-faint` (#25).

## Impact

- Новый пакет `internal/mrlinks` (зависит только от `internal/gitlab`, `golang.org/x/net/html`, `github.com/charmbracelet/x/ansi` — оба уже в `go.mod`).
- `internal/gitlab` (`Link`, поля `MergeRequest`), `internal/gitlab/graphql` (`MRDetail`, маппинг), `internal/gitlab/fake` (ссылки в деталях).
- `internal/config` (`Links`, проверка), `cmd/glview` (передача правил в `Env`).
- `internal/ui/app` (правила в `Env`), `internal/ui/common` (оверлей списка ссылок на рамке `ListModal`), `internal/ui/screens/mrdetail`, `internal/ui/screens/project`, `internal/ui/screens/workspace`, `internal/i18n`.
- Документы: docs/SPEC.md (см. design.md, решение 7).
- #23: `internal/gitlab` (`TagService`, `Comparison`, `MatchTagPipelines`, `WorkspaceService.Tags`), `gitlab/rest` (теги, compare, пайплайны тегов, workspace), `gitlab/hybrid`, `gitlab/fake`; новый `internal/semver`; `ui/screens/project` (таб Tags, `Options.TagSearch`), `ui/screens/workspace` (таб Tags), новые `ui/screens/newtag`, `ui/screens/tagactions`; `internal/i18n`. Документы: docs/SPEC.md (design.md, решение 14).
- #24: `internal/gitlab` (`IssueService`, `IssueFilter`, `RelatedMR`, поля `Issue`, `MergeRequestService.Comment`, `ProjectService.Milestones`), `gitlab/rest` (issues, notes, milestones), `gitlab/hybrid`, `gitlab/fake`; `ui/screens/project` (таб Issues, подсказки milestones, `c` в MRs), `ui/screens/workspace` (`c` в MRs), `ui/screens/mrdetail` (переход на `threads`, `c`); новые `ui/screens/threads`, `ui/screens/issuedetail`, `ui/screens/comment`, `ui/screens/issueactions`; `ui/app` (черновики комментариев в `Env`); `internal/i18n`. Документы: docs/SPEC.md (design.md, решение 21).
- #25: `internal/ui/common` (`Theme`, `Styles`), `internal/config` (проверка `theme`), `cmd/glview` (выбор темы), `gitlab/httpx` и маппинг ошибок (только при найденных расхождениях), `ui/app` (офлайн/401 — при расхождениях), тесты экранов 80×24; README.md. Документы: docs/SPEC.md (design.md, решение 34).
