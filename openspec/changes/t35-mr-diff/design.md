## Context

Задача #27 (T35), change отдельная (после MVP одна change — одна задача, чтобы задачи шли параллельно). Поведение — specs/ этой change, полная картина — docs/SPEC.md §3.5 («Diff», «Переход с Diff»), §3.16 («К строке кода»), §4, §5.2, §11.2, §13.1 п. 20. SPEC уже поправлен, код — по нему.

Что уже есть и переиспользуется:
- `mrdetail.Screen`: вкладки (`tabOverview…tabCommits`, `tabCount`), `setTab`, `tabBar`, ленивая вкладка Commits (`commitsTab`: `opened`, `loading`, `reqSHA`, `refresh`, перезапрос по `r` после ответа деталей и при смене head SHA) — образец загрузки диффа; `scrollKey`, `window`, `bodyHeight`.
- Discussions: `threads.Model` (`screens/threads`) — текущий тред, сворачивание, прокрутка к треду; на него опирается «Переход с Diff».
- `screens/comment`: `Target`, `Open`, черновики по ключу, `Invalidate`, `SentMsg` — форма комментария; MR detail после `SentMsg` перезапрашивает discussions.
- `rest/mrs.go`: маппинг discussions уже читает `n.Position.NewPath/NewLine/OldPath/OldLine` (сейчас сводит в `Path/Line`) — расширяется без нового запроса.
- `graphql`: фрагмент `MRDetail`, маркеры `#@opt … #@end` для полей, которых может не быть на старом инстансе.
- `joblog`: образец пакета разбора без UI с unit-тестами; горизонтальный скролл `←/→` и номера строк Muted — в `screens/joblog`.
- `common.Styles` — роли темы (Success, Error, Muted, Selected); экраны цвета не знают.
- `app.OpenURL`, `app.Copy`, `Env.Fail`, `common.Gen`/`Token`.
- Табы MRs: `project/mrs.go` и `workspace/mrs.go` — обработка клавиш строки MR и открытие `mrdetail.New`.

## Goals / Non-Goals

**Goals:** читать дифф MR в терминале 80×24 и шире, видеть diff-треды на своих строках, оставлять комментарий к строке, открыть MR сразу на Diff из списка.

**Non-Goals:** side-by-side, подсветка синтаксиса, ответы в треды и resolve, дифф отдельного коммита и версий MR, многострочные комментарии (`line_range`), suggestions (SPEC §1).

## Decisions

1. **Разбор — в `internal/diff`.** `Parse(hunks string) []Line`, `Line{Kind (Hunk|Context|Add|Del|NoNewline), Old, New int, Text string}`; номера из заголовка `@@ -a,b +c,d @@`, у добавленной `Old = 0`, у удалённой `New = 0`; `\ No newline at end of file` — свой вид без номеров. `Find(lines, pos NotePosition) int` — индекс строки треда: по `NewLine` (у добавленной и контекстной), по `OldLine`, если `NewLine == 0`; -1 — не нашли. Табы → 4 пробела делает UI, не парсер. Почему отдельный пакет: тестируется без UI, как `joblog`.
2. **Модель вкладки.** Плоский список видимых строк (`row{kind: fileHeader|hunk|code|noNewline|thread|note, file, line, thread}`) пересобирается при загрузке, сворачивании, смене discussions, ширины и языка; `View` рисует только окно. Курсор — индекс в этом списке; при пересборке держится по ключу (`path` + `Old`/`New` строки или ID треда), иначе — на заголовке того же файла, иначе — по индексу с ограничением.
3. **Сворачивание** — `map[path]bool` (выбор пользователя) поверх умолчаний (`Generated`, `TooLarge` — свёрнуты). `z`: если хоть один развёрнут — свернуть все, иначе развернуть все.
4. **Горизонтальный скролл** — общий сдвиг `xoff` (шаг 8), применяется к тексту кода (`ansi.Cut` по видимой ширине), номера и знак не сдвигаются. Строки треда и заголовки не сдвигаются.
5. **Треды на строках** — из уже загруженных `s.discs`: diff-треды (`Position != nil`) раскладываются по `(path, index)` через `diff.Find`; не найденные пропускаются. Строка треда: `●`/`✓`, автор, время, первая строка тела первой заметки (без markdown — `xansi.Strip` от простого текста, переводы строк — пробел), `N ответов` (плюрализация как в Discussions).
6. **Переход с Diff** — `enter` на строке треда: `setTab(tabDiscussions)`, `threads` делает текущим этот тред (по ID), разворачивает и прокручивает к нему. Нужен метод в `threads.Model` (`Focus(id string)`), если его нет.
7. **Загрузка** — копия схемы Commits (`diffTab`: `list`, `loaded`, `loading`, `err`, `opened`, `reqSHA`, `refresh`), ключ кэша `diffs:<path>:<iid>:<sha>`, TTL 10 мин. Ответ `/diffs` SHA не несёт: сверка «ответ под текущий SHA» не нужна, достаточно ключа кэша. Тики поллинга дифф не трогают.
8. **Комментарий к строке** — `comment.Target` получает `Position *gitlab.DiffPosition` и `Where string` (`path:line` для заголовка); `submit` вызывает `CommentLine` при `Position != nil`, иначе `Comment`. Ключ черновика включает `path`, `OldLine`, `NewLine`. Позиция: `DiffRefs` из `s.mr.DiffRefs`, `OldPath/NewPath` файла, у добавленной — `NewLine`, у удалённой — `OldLine`, у контекстной — оба. `DiffRefs == nil` или строка не кода — обычный комментарий. Ответ `CommentLine` — `Discussion`, но UI после `SentMsg` всё равно перезапрашивает discussions (как сейчас).
9. **`diffRefs` в GraphQL** — поле `diffRefs { baseSha headSha startSha }` в `MRDetail`; если на инстансе его нет, — `#@opt`, тогда `DiffRefs == nil` и `c` на Diff — обычный комментарий.
10. **REST `/diffs`** — client-go `MergeRequests.ListMergeRequestDiffs` (если в версии client-go его нет — прямой запрос через клиент), `per_page=100`, все страницы. `too_large`/`collapsed` → `TooLarge`; `generated_file` → `Generated` (поля может не быть — false). 400 на `POST /discussions` → ошибка с сообщением сервера (как 400 у тегов).
11. **Ссылка `o`/`y`** — `<MR.WebURL>/diffs#<hex(sha1(NewPath))>`; на пустом диффе — `<MR.WebURL>/diffs`.
12. **Стили** — в `common.Styles` добавить `DiffAdd` (Success), `DiffDel` (Error), `DiffHunk` (Muted); номера — `Muted`; заголовок файла — `Bold`. Без цвета знаки `+`/`-` остаются.
13. **`d` в табах MRs** — `mrdetail.New` получает начальную вкладку (опция или отдельный конструктор `NewOnTab`); в workspace — на строке MR (на заголовке группы и «ещё N» не действует), в футере и хелпе — `d diff`.

## Risks / Trade-offs

- [Большой дифф (тысячи строк)] → список строк строится один раз на загрузку/ширину, рисуется окно; сервер сам скрывает слишком большие файлы (`too_large`).
- [Номер строки треда на устаревшем диффе] → тред на Diff не показывается, в Discussions есть; не пытаемся сопоставлять старые версии. Версию определяет `head_sha` позиции против `DiffRefs.HeadSHA` MR: совпадение номера строки у другой версии показало бы тред под чужим кодом.
- [Нет `diffRefs` на старом инстансе] → `c` на Diff — обычный комментарий, без ошибки.
- [Параллельная работа T36/T37 трогает `client.go`, фейк, i18n] → новые методы и строки — в отдельных файлах (`rest/diffs.go`, `fake/diffs.go`, `i18n/*_t35.go`), в общих файлах — только добавления в конец интерфейсов и структур.
