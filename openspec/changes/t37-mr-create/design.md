## Context

Задача #50 (T37), change отдельная (после MVP одна change — одна задача, задачи идут параллельно). Поведение — specs/ этой change, полная картина — docs/SPEC.md §3.18, §5.2, §6.3, §11.2, §13.1 п. 22. SPEC уже поправлен, код — по нему.

Что уже есть и переиспользуется:
- `screens/runpipeline` и `screens/newtag` — свои контролы форм: поле ref с подсказками (debounce 300 мс, окно подсказок, `↑/↓`, `enter` подставить), фокус по кругу `tab`/`shift+tab`, кнопка в фокусе, снимок при `enter` на кнопке, встроенный confirm, блокировка на время запроса, `common.Gen` экземпляра формы, строка ошибки над кнопкой; golden и fit-тесты 80×24.
- `screens/comment` — `textarea` (bubbles v2) и черновик в памяти сессии по ключу.
- `project/suggest.go` — источники подсказок `membersSource`, `labelsSource`, milestones (кэш 5 мин).
- `ProjectService.Branches/Members/Labels/Milestones`, `TagService.Compare` → `Comparison`, `MergeRequests().List` с `MRFilter.SourceBranch` (превью «Проектов» workspace).
- `gitremote.Repo.Branch`/`Detached` (локальная ветка), `localgit.Tracker.AheadBehind(dir)` (↑ без запуска git).
- `app.InputCapturer` (глобальные хоткеи выключены), `app.Push`/`PopMsg`, `app.NewConfirm`, `Env.Fail`, `Env.User`, инвалидация кэша списков MR (как после merge/approve в `mractions`).
- `mrdetail.New` — экран нового MR.

## Goals / Non-Goals

**Goals:** создать MR из TUI со всеми полями `POST /merge_requests`, кроме форков; не дать создать бессмысленный MR (нет ветки на сервере, нет изменений, дубль).

**Non-Goals:** MR из форка и `allow_collaboration` (#56), push ветки, редактирование созданного MR, `Resolve "<issue>"` по номеру issue в имени ветки.

## Decisions

1. **Экран на стеке, `CapturesInput() == true` всегда** (SPEC §13.1 п. 22). `esc` — закрыть (черновик сохраняется), кроме момента создания.
2. **Поля — свои контролы**, общий «текст + подсказки» вынести из `runpipeline`/`newtag`, если это не ломает их тесты; иначе — свой небольшой компонент в `newmr`. Мульти-поле (assignees/reviewers/labels): `[]item` чипами + текстовое поле + подсказки, `backspace` в пустом поле — убрать последний.
3. **Состояние веток** — один «проверочный» запрос на пару веток через 300 мс после изменения: сначала `Branches("^src$")`; если ветка есть (или запрос упал по сети) — параллельно `Compare(target, src)` и `MergeRequests().List{State: opened, SourceBranch: src, TargetBranch: target}`; ошибка списка MR — строка состояния «не удалось проверить», создание не блокируется; результат с ключом `src|target` и `Gen`; ответы для другой пары выбрасываются. Незапушенные коммиты — из `Tracker.AheadBehind(dir)`, только если `src` — локальная ветка этой папки.
4. **Заголовок по умолчанию** — одиночный коммит сравнения → его заголовок, иначе `humanize(src)`: `-`/`_` → пробел, первая руна — заглавная. Флаг «правился» снимает следование. Draft-префикс добавляется только в снимок при создании.
5. **Шаблоны** — список при открытии (кэш 5 мин); `Default` (без учёта регистра) — выбрать и подставить в пустое описание; флаг «описание правилось после подстановки» решает, нужен ли confirm при смене шаблона.
6. **Умолчания флажков** — из `Projects().Get` (REST, поля `remove_source_branch_after_merge`, `squash_option`); пока не пришёл — `remove = true`, `squash = false`, флажки редактируемы. `always`/`never` — флажок заблокирован с подписью.
7. **Снимок и создание.** `enter` на кнопке → локальные проверки → ожидание состояния веток для снимка (если ещё нет) → ветки нет / 0 коммитов — ошибка; есть открытый MR — confirm «Открыть его?» (по умолчанию «Да») → `MergeRequests().Create(snapshot)`. Успех: `Invalidate` списков MR проекта и групп workspace, черновик стирается, тост `MR !N создан`, форма снимается (`PopMsg`) и кладётся `mrdetail.New` с ответом `Create`. Ошибка — тост с текстом сервера, форма разблокируется.
8. **Черновик** — `map[host|project]formState` в памяти (пакетная переменная или `Env`, как в `comment`), восстанавливается по `n`; запросы проверок перезапускаются.
9. **`Milestones` → `[]Milestone`.** Меняется сигнатура: REST отдаёт `ID` и `Title`, фильтр issues и подсказки берут `Title`. Тесты issues обновляются, поведение не меняется.
10. **`Comparison.Files`** — `len(cmp.Diffs)` в REST `Compare`; теги на него не смотрят.
11. **REST `Create`** — client-go `MergeRequests.CreateMergeRequest` (`AssigneeIDs`, `ReviewerIDs`, `Labels` как `LabelOptions`, `MilestoneID`, `RemoveSourceBranch`, `Squash`); ответ → `MergeRequest` (минимум: IID, Title, WebURL, ветки, State); 409/400/422 → ошибка с сообщением сервера.
12. **Раскладка 80×24** — одна колонка, подписи слева фиксированной ширины (по длинной подписи текущего языка); форма прокручивается так, чтобы поле в фокусе и его подсказки были видны; описание — от 3 строк до трети высоты.

## Risks / Trade-offs

- [Много полей на 24 строках] → прокрутка формы; golden 80×24 с фокусом в середине и внизу формы.
- [Гонка проверок и ввода веток] → ключ `src|target` + `Gen`; снимок ждёт ответ именно своей пары.
- [Сигнатура `Milestones`] → правка вызовов в одном коммите с тестами issues.
- [Параллельная работа T35/T36 трогает `client.go`, `MergeRequestService`, фейк, i18n] → новые файлы (`rest/mrcreate.go`, `fake/mrcreate.go`, `i18n/*_t37.go`), в общих — только добавления.
