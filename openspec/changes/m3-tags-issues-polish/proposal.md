## Why

M2 закрыт (архив `2026-10-22-m2-workspace`). Веха «M3 — теги, issues, формы, полировка» добивает MVP. Эта change — общая на веху: задачи вехи добавляются в неё по мере начала работы. Первая задача — #51 (T34). Дорожная карта — #36.

Сейчас заголовок MR — простой текст, кликабельны только markdown-ссылки в описании (glamour выводит их OSC 8). Ссылки на `#12`, `!45`, `@anna.k` и ключи трекера вида `ABC-123` в заголовке и описании не открываются. В tmux без `terminal-features hyperlinks` и в терминалах без OSC 8 не открываются вообще никакие ссылки.

Теги (#23, T23): табы Tags проекта и workspace — заглушки «появится позже». Нельзя посмотреть теги с пайплайнами, создать тег со следующей версией и удалить тег без браузера — а это частое действие при релизе (SPEC §1, цель 3).

Задачи вехи: #51 (T34) — сделана; #23 (T23) — в работе; #24 (T24), #25 (T25) — добавятся в эту change позже.

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

## Capabilities

### New Capabilities
- нет

### Modified Capabilities
- `merge-requests`: ссылки MR — источники, OSC 8 в title и описании, оверлей `L` в MR detail.
- `project-view`: OSC 8 в title списка MR, `L` в табе MRs; таб Tags, предложение версии, форма New tag, удаление тега (#23).
- `workspace`: то же в групповом табе MRs; групповой таб Tags, свёрнутые группы и `P` для Tags, `f` → Tags (#23).
- `configuration`: правила `links`.

## Impact

- Новый пакет `internal/mrlinks` (зависит только от `internal/gitlab`, `golang.org/x/net/html`, `github.com/charmbracelet/x/ansi` — оба уже в `go.mod`).
- `internal/gitlab` (`Link`, поля `MergeRequest`), `internal/gitlab/graphql` (`MRDetail`, маппинг), `internal/gitlab/fake` (ссылки в деталях).
- `internal/config` (`Links`, проверка), `cmd/gitlab-tui` (передача правил в `Env`).
- `internal/ui/app` (правила в `Env`), `internal/ui/common` (оверлей списка ссылок на рамке `ListModal`), `internal/ui/screens/mrdetail`, `internal/ui/screens/project`, `internal/ui/screens/workspace`, `internal/i18n`.
- Документы: docs/SPEC.md (см. design.md, решение 7).
- #23: `internal/gitlab` (`TagService`, `Comparison`, `MatchTagPipelines`, `WorkspaceService.Tags`), `gitlab/rest` (теги, compare, пайплайны тегов, workspace), `gitlab/hybrid`, `gitlab/fake`; новый `internal/semver`; `ui/screens/project` (таб Tags, `Options.TagSearch`), `ui/screens/workspace` (таб Tags), новые `ui/screens/newtag`, `ui/screens/tagactions`; `internal/i18n`. Документы: docs/SPEC.md (design.md, решение 14).
