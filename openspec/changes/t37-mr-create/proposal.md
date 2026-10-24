## Why

Задача #50 (T37) после MVP. MR создаётся в браузере или через `glab mr create` с редактором; из TUI, где уже видны ветка, MR и пайплайны, создать MR нельзя. В MVP создание MR было в «Не-целях» (SPEC §1). Пункт `mr create` из #30 заменяется этой задачей. Дорожная карта — #36.

## What Changes

- `gitlab`: `NewMR`, `Milestone`; `MergeRequestService.Create`; `ProjectService.Milestones` возвращает `[]Milestone` (вызовы фильтров issues берут `Title`), `MRTemplates`, `MRTemplate`; `Project.RemoveSourceBranchAfterMerge`/`SquashOption`; `Comparison.Files` (#50).
- REST: `POST /projects/:id/merge_requests`, `GET /templates/merge_requests` (+ `/:name`), поля проекта, `diffs` в compare; `hybrid` делегирует; фейк (#50).
- Экран-форма New MR (`screens/newmr`): ветки с подсказками, строка состояния веток (сравнение, ветка на сервере, незапушенные коммиты, открытый MR из ветки), заголовок по умолчанию, Draft, шаблоны описания, описание, assignees/reviewers/labels чипами с подсказками, milestone, флажки по настройкам проекта, предпросмотр, кнопка `enter Создать`, проверки, снимок, блокировка, черновик (#50).
- `n` в табе MRs проекта и в групповом табе MRs workspace; после создания — MR detail нового MR, сброс кэшей списков MR (#50).
- Строки — i18n en/ru (#50).
- SPEC уже поправлен до кода: §1, §2.6, §3.1, §3.3, §3.4, §3.14, §3.18, §4, §5.2, §5.8, §6.3, §10.5, §11.2, §13.1 п. 22 (#50). MR из форка — #56.

## Capabilities

### New Capabilities
- нет

### Modified Capabilities
- `merge-requests`: форма New MR и создание MR (#50).
- `project-view`: `n` в табе MRs (#50).
- `workspace`: `n` в групповом табе MRs (#50).

## Impact

- Код: `internal/gitlab` (types, client), `gitlab/rest` (`mrcreate.go`, `refs.go`, `client.go`), `gitlab/hybrid`, `gitlab/fake`, `internal/ui/screens/newmr` (новый), `screens/project/mrs.go`, `screens/project/suggest.go` (milestones), `screens/workspace/mrs.go`, `internal/i18n`.
- API: REST `POST /merge_requests`, шаблоны MR, `GET /projects/:id` с новыми полями.
- Зависимостей не добавляется.
