## Why

MVP закрыт (M3, `2026-10-24-m3-tags-issues-polish`). Первая задача после MVP — #27 (T35): в MR detail нельзя посмотреть изменения и оставить комментарий к строке — за этим приходится идти в браузер, хотя ревью — самое частое действие с MR. В макете `MRDetail` вкладка Diff есть, в MVP она была убрана (SPEC §1, «Не-цели»). Дорожная карта — #36.

## What Changes

- Пакет `internal/diff`: разбор ханков unified diff в строки (вид, номера старой и новой строки, текст), поиск строки по позиции diff-треда (#27).
- `gitlab`: `DiffFile`, `DiffRefs`, `DiffPosition`, `MergeRequest.DiffRefs`, расширенная `NotePosition` (`OldPath/NewPath/OldLine/NewLine`); `MergeRequestService.Diffs` и `CommentLine` (#27).
- REST: `GET /merge_requests/:iid/diffs` (все страницы), `POST /merge_requests/:iid/discussions` с позицией; GraphQL `MRDetail`: `diffRefs{baseSha headSha startSha}`; фейк (#27).
- MR detail: вкладка Diff между Overview и Discussions — файлы одной прокруткой, номера строк, `+`/`-` ролями, горизонтальный скролл, сворачивание файлов, `J/K` по файлам, треды на строках, `enter` → тред в Discussions, `c` — комментарий к строке, `o`/`y` — файл в вебе; ленивая загрузка по правилам Commits (#27).
- Форма комментария (3.16): цель с позицией строки, заголовок `!479 · path:42`, черновик по ключу с позицией (#27).
- `d` в табе MRs проекта и групповом табе MRs workspace — MR detail на вкладке Diff (#27).
- `common.Styles`: стили строк диффа (добавленная, удалённая, заголовок ханка) по ролям темы (#27).
- Строки — i18n en/ru (#27).
- SPEC уже поправлен до кода: §1, §2.6, §3.1, §3.3, §3.4, §3.5 («Diff»), §3.16, §4, §5.2, §5.8, §6.1, §6.3, §11.2, §13.1 п. 20 (#27).

## Capabilities

### New Capabilities
- нет

### Modified Capabilities
- `merge-requests`: вкладка Diff, треды на строках, комментарий к строке; список вкладок MR detail (#27).
- `project-view`: `d` в табе MRs (#27).
- `workspace`: `d` в групповом табе MRs (#27).

## Impact

- Код: `internal/diff` (новый), `internal/gitlab` (types, client), `gitlab/rest`, `gitlab/graphql` (фрагмент `MRDetail`), `gitlab/hybrid`, `gitlab/fake`, `internal/ui/screens/mrdetail`, `screens/comment`, `screens/project/mrs.go`, `screens/workspace/mrs.go`, `ui/common/styles.go`, `internal/i18n`.
- API: новые REST-эндпоинты `/diffs` (GitLab ≥ 15.7, минимум проекта — 16.0) и `/discussions` (POST).
- Зависимостей не добавляется.
