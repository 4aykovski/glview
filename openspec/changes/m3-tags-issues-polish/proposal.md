## Why

M2 закрыт (архив `2026-10-22-m2-workspace`). Веха «M3 — теги, issues, формы, полировка» добивает MVP. Эта change — общая на веху: задачи вехи добавляются в неё по мере начала работы. Первая задача — #51 (T34). Дорожная карта — #36.

Сейчас заголовок MR — простой текст, кликабельны только markdown-ссылки в описании (glamour выводит их OSC 8). Ссылки на `#12`, `!45`, `@anna.k` и ключи трекера вида `ABC-123` в заголовке и описании не открываются. В tmux без `terminal-features hyperlinks` и в терминалах без OSC 8 не открываются вообще никакие ссылки.

Задачи вехи: #51 (T34) — в работе; #23 (T23), #24 (T24), #25 (T25) — добавятся в эту change позже.

## What Changes

- Пакет `internal/mrlinks`: `<a href>` из HTML GitLab, правила `links` из конфига, голые `http(s)://`, общий список без дублей в порядке текста, наложение OSC 8 на уже отрисованный текст без изменения видимого текста (#51).
- `gitlab.Link`, `MergeRequest.TitleLinks/DescriptionLinks`; GraphQL `MRDetail` запрашивает `titleHtml descriptionHtml` (опциональные поля `#@opt`), `MRRow` не меняется (#51).
- Конфиг: раздел `links: [{pattern, url}]`, проверка правил с тостом-предупреждением (#51).
- MR detail: title и описание на Overview со ссылками OSC 8; таб MRs (проект и workspace): Title и превью — OSC 8 по правилам и голым URL (#51).
- Хоткей `L` в MR detail и в табе MRs: оверлей «текст — URL», `enter` открыть, `y` копировать, `esc` закрыть; в табе MRs детали MR догружаются по `L`; нет ссылок — тост (#51).
- Строки — i18n en/ru (#51).
- Правки SPEC до кода: §3.1, §3.4, §3.5 («Ссылки MR»), §4, §5.2, §6.1, §7, §13.1 (#51).

## Capabilities

### New Capabilities
- нет

### Modified Capabilities
- `merge-requests`: ссылки MR — источники, OSC 8 в title и описании, оверлей `L` в MR detail.
- `project-view`: OSC 8 в title списка MR, `L` в табе MRs.
- `workspace`: то же в групповом табе MRs.
- `configuration`: правила `links`.

## Impact

- Новый пакет `internal/mrlinks` (зависит только от `internal/gitlab`, `golang.org/x/net/html`, `github.com/charmbracelet/x/ansi` — оба уже в `go.mod`).
- `internal/gitlab` (`Link`, поля `MergeRequest`), `internal/gitlab/graphql` (`MRDetail`, маппинг), `internal/gitlab/fake` (ссылки в деталях).
- `internal/config` (`Links`, проверка), `cmd/gitlab-tui` (передача правил в `Env`).
- `internal/ui/app` (правила в `Env`), `internal/ui/common` (оверлей списка ссылок на рамке `ListModal`), `internal/ui/screens/mrdetail`, `internal/ui/screens/project`, `internal/ui/screens/workspace`, `internal/i18n`.
- Документы: docs/SPEC.md (см. design.md, решение 7).
