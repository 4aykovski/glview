## Context

M0–M2 закрыты (архивы `2026-10-19-m0-foundation`, `2026-10-20-m1-project-mode`, `2026-10-22-m2-workspace`). Change общая на веху M3; в ней #51 (T34, сделана), #23 (T23, теги, сделана) и #24 (T24, issues и комментарии). Мотивация — proposal.md, поведение — specs/ этой change, полная картина — docs/SPEC.md §3.5 («Ссылки MR»), §7 — для T34; §3.3, §3.4, §3.10, §6.3, §11.2 — для T23; §3.4 (Issues), §3.15, §3.16, §5.2, §6.3 — для T24.

Что уже есть и переиспользуется:
- `common.Markdown` (glamour v2): markdown-ссылки и автоссылки уже выводятся с OSC 8. `[#12](url)` glamour рисует как `#12 url` — текст и URL рядом (опции «только текст» у glamour нет, `SkipHref` выставляется только для таблиц).
- `common.HTMLToMarkdown` на `golang.org/x/net/html` — пример разбора HTML.
- `graphql.Client.DoOpt` и маркеры `#@opt … #@end` — опциональные поля схемы (`commitCount`).
- `common.ListModal`, `ListMove`, `ListPage`, `ListStart` — рамка списочных оверлеев `p`, `F`, `P`.
- `app.OpenURL`, `app.Copy` (OSC 52 + тост `Copied(what)`), `Env.Host`.
- `config.Warning` / `WarnValue` и тосты предупреждений конфига при старте.
- `lipgloss.Style.Hyperlink`, `xansi.SetHyperlink/ResetHyperlink`, `xansi.Strip` (golden-снимки сравнивают текст без ANSI, OSC 8 на них не влияет).

Для T23 (теги):
- `gitlab.TagService` с одним `List` (M1, автодополнение ref в Run pipeline и в фильтрах), REST `tags.List` в `rest/refs.go`, `gitlab.Tag`/`ReleaseSummary`/`CommitSummary` в `types.go`.
- `rest.mapPipeline`, `rest.mapCommit`, `rest.mapError`; `hybrid.latestTag` и семафор `tagSem` (≤ 4 REST на проект) в обзоре workspace.
- Табы-заглушки `project/placeholder.go`, `workspace/placeholder.go` (Tags), `project.Options.Tab`.
- Шаблон таба списка — `project/pipelines.go` (страницы, подгрузка, поллинг 30 с, ленивое превью по выбранной строке с кэшем); шаблон группового таба — `workspace/groupBase[T]` (свёрнутые группы, `state.json` по имени таба, `P`, режим фильтра, `listen`).
- `pipeactions` — шаблон пакета действий над строкой (confirm → мутация → `DoneMsg` → инвалидация кэша и тост); `runpipeline.Form` — шаблон формы-оверлея: `common.Gen` экземпляра, встроенный confirm (`f.confirm`), блокировка на время запроса, кнопка в фокусе.
- `app.NewConfirm`, `app.OpenOverlay`, `app.OpenURL`, `app.Copy`, `Env.Fail`, i18n `Toast.TagCreated`, `Plural.Tags`, `WS.NotApplicableToTags`.

Для T24 (issues и комментарии):
- `gitlab.Issue`, `gitlab.Note`, `gitlab.Discussion` в `types.go`; `IssueService` в SPEC §6.3 описан, в коде его нет; `Client.Issues()` нет. `MergeRequestService.Comment` в коде нет.
- Таб MRs проекта (`project/mrs.go`) — шаблон таба Issues: строка фильтров `filterline` с ключами `key:value`, подсказки `membersSource`/`labelsSource` (`project/suggest.go`), `s`/`m`/`backspace`, URL `o` по фильтрам, превью справа/снизу (`previewMinW = 100`).
- Карточки тредов MR — методы `mrdetail.Screen` в `mrdetail/threads.go` (`noteMarkdown`, `noteHead`, `folded`, `cardInner`, `foldedLine`, `layout`, `threadKey`); снаружи не переиспользуются.
- `common.Markdown`, `common.HTMLToMarkdown` — тело заметок и описание.
- Шаблоны формы: `runpipeline.Form` и `newtag` (overlay, `common.Gen`, кнопка в фокусе, блокировка на время запроса); `bubbles/v2/textarea`.

## Goals / Non-Goals

**Goals (T34):**
- Ключ трекера, `#12`, `!45`, `@user` и URL в title и описании MR открываются cmd/ctrl+кликом в MR detail.
- Тот же набор ссылок доступен с клавиатуры (`L`) там, где OSC 8 не работает.
- Без новых запросов при обычном просмотре списка; без роста сложности `MRRow`.

**Goals (T23):**
- Таб Tags проекта и workspace по SPEC §3.3, §3.4: список, поиск, пайплайны тегов, превью с compare.
- Создание тега формой New tag с предложением версии (`internal/semver`) и confirm; удаление `D` с confirm.
- Нагрузка workspace — как у MRs/Pipelines: запросы только по раскрытым группам.

**Goals (T24):**
- Таб Issues проекта по SPEC §3.4 и макету `Issues`: фильтры, связанный MR, превью с описанием и последними комментариями.
- Полноэкранный просмотр issue с тредами-карточками, как в MR detail (общий код карточек).
- Один компонент комментария для `c` в Issues (таб и экран issue) и в MR (таб MRs проекта и workspace, MR detail).
- `x` close (confirm) / reopen.

**Non-Goals (T34):**
- Ссылки в тредах Discussions (там markdown-ссылки glamour, как сейчас).
- Ссылки в title списка MR из HTML GitLab (`#12` и т. п. в списке не кликабельны — только правила и голые URL).
- Разбор ссылок GitLab на клиенте (`#12` → URL issue) — источник истины HTML GitLab: он знает, существует ли объект, и про интеграции трекеров.
- Ссылки в issues (#24) — позже, тем же пакетом.

**Non-Goals (T24):**
- Создание issue, правка меток, assignee, milestone, ветка+MR из issue (SPEC §1, «Отличия от макетов»: `n`, `L`, `A`, `b` убраны).
- Ответ в тред, resolve, редактирование и удаление комментариев; комментарии к строкам кода.
- Таб Issues в workspace (в SPEC его нет).
- Предпросмотр markdown в форме комментария.
- Ссылки OSC 8 в issues (пакет `mrlinks` — позже).

**Non-Goals (T23):**
- Релизы: создание, правка, просмотр release notes целиком (в превью — только первая строка описания релиза).
- Protected tags: управление защитой; клиент не проверяет права заранее — решает ответ GitLab.
- Подписанные теги, правка сообщения существующего тега.
- Клиентская сортировка: порядок списка — `order_by=version&sort=desc` сервера, страницы не пересортировываются.

## Decisions

### 1. Источник ссылок GitLab — `titleHtml`/`descriptionHtml` только в `MRDetail`
Поля добавляются в `MRDetail`, каждое своим `#@opt`-сегментом (`#@opt titleHtml` / `#@opt descriptionHtml`). В `MRRow` — нет: список и workspace-батчи и так упираются в лимит сложности 250 (SPEC §5.2), markdown-поля у GitLab дороже обычных. Таб MRs получает ссылки GitLab лениво — по `L` запрашивает детали выделенного MR тем же `MergeRequests().Get`, что и MR detail (новый метод интерфейса не нужен, кэш деталей переиспользуется).

Разбор HTML — `mrlinks.FromHTML(html, origin string) []gitlab.Link` на `x/net/html`, вызывается в маппинге `graphql`. Берутся все `<a href>`; текст — весь текст внутри `<a>` (сущности раскрыты, пробелы схлопнуты, обрезаны). Пропуск: пустой текст, `href` с `#`, схемы кроме `http`/`https`. Относительный `href` (начинается с `/`) — `origin + href`, где `origin` — схема и хост инстанса (`Client.origin()`); path-prefix GitLab уже пишет в `href`. Порядок — порядок в HTML, дубли не убираются (это делает сборка, решение 3).

### 2. Правила конфига и голые URL
`config.Links []LinkRule{Pattern, URL string}` (`yaml:"links"`). Проверка при загрузке: пустой `pattern`/`url`, ошибка `regexp.Compile`, `url` не с `http://`/`https://` → правило выкидывается, `Warning` с ключом `links[i].pattern` (или `links[i].url`), тост при старте, как у прочих предупреждений. Компилированные правила — `mrlinks.Rule{Re *regexp.Regexp, URL string}`, кладутся в `app.Env` при сборке в `cmd`.

Совпадение правила: `Re.FindAllStringSubmatchIndex` по тексту, URL — `Re.ExpandString(nil, rule.URL, text, match)`, текст ссылки — всё совпадение. Голые URL — `https?://[^\s<>"'` + "`" + `]+` с обрезкой хвоста: `.,;:!?` и закрывающие `)]}>` без парной открывающей внутри ссылки.

### 3. Сборка общего списка — `mrlinks.Collect`
```go
type Source struct {
    Title, Description           string
    TitleLinks, DescriptionLinks []gitlab.Link // из HTML; nil для MRRow
}
func Collect(src Source, rules []Rule) []gitlab.Link
```
Порядок: блок title, потом блок description. В блоке: ссылки HTML, затем совпадения правил по порядку правил, затем голые URL; всё стабильно сортируется по позиции первого вхождения текста в исходный текст блока (не найденные — в конце, исходный порядок). Дубли по URL — остаётся первая. Одинаковый текст с разными URL остаётся в списке оба раза (это разные ссылки), но для наложения OSC 8 текст берёт первую ссылку списка (HTML важнее правил, так как идёт раньше при одинаковой позиции).

### 4. OSC 8 — наложение на отрисованный текст, а не подмена в markdown
Подмена `ABC-123` → `[ABC-123](url)` до glamour дала бы `ABC-123 https://…` прямо в тексте описания — текст меняется. Поэтому glamour рендерит описание как есть, а `mrlinks.Apply(rendered string, links []gitlab.Link) string` накладывает OSC 8 на результат:
- строка разбирается по строкам (`\n`); для каждой строки строится видимый текст и отображение позиций видимых рун в байты исходной строки (парсер `x/ansi`, CSI/OSC пропускаются);
- участки видимого текста внутри уже открытого OSC 8 (от `ESC]8;…;url` до `ESC]8;;`) помечаются занятыми;
- для каждой ссылки ищутся все вхождения `Text` целым токеном (руна до и после — не `unicode.IsLetter/IsDigit` и не `_`); при пересечении берётся более длинное, при равной длине — раньше в списке;
- перед первой руной вхождения вставляется `xansi.SetHyperlink(url)`, после последней — `xansi.ResetHyperlink()`; стили SGR между ними не трогаются (OSC 8 и SGR независимы).
Видимый текст не меняется: `xansi.Strip(Apply(s, l)) == xansi.Strip(s)` — свойство в тестах. Ссылки, разорванные переносом glamour или обрезанные `…`, не находятся — это допустимо, они в оверлее `L`.

### 5. Где накладывается
- **MR detail:** title блока заголовка — `Apply(styledTitle, Collect(все источники))`; описание на Overview — `Apply(common.Markdown(...), links)` до разбиения на строки для прокрутки. Результат кэшируется вместе с отрисованным описанием (пересчёт при смене MR/ширины, как сейчас).
- **Таб MRs** (`project/mrs.go`, `workspace/mrs.go`) и их превью: `Collect(Source{Title, Description}, rules)` — без HTML. В таблице OSC 8 накладывается на текст ячейки после обрезки по ширине (ячейка может закончиться `…` — частичное вхождение не матчится, незакрытых OSC 8 не бывает). Как встроить в `common/table` (хук рендера ячейки или предварительно обрезанный текст) — на усмотрение реализации; требование — сбалансированные OSC 8 и неизменная ширина ячейки.
- **Discussions** — без изменений.

### 6. Оверлей `L`
Модель оверлея в `ui/common` (рядом с `listmodal.go`, например `linksmodal.go`): `[]gitlab.Link`, выделение, заголовок `Links · !479`; рисуется `ListModal` с пустой строкой ввода (или без неё — если нужна функция-вариант `ListModal` без input, добавить её, не ломая `p`/`F`/`P`). Строка: `text — url` (`url` — `Styles.Muted`), если `text == url` — только `url`; обрезка `…` по ширине. Клавиши — как в SPEC §3.5; `enter` → `app.OpenURL` + закрыть, `y` → `app.Copy(url, msg.What.Link)` + закрыть. Обновление MR при открытом оверлее (`SetLinks`) сохраняет выделение по URL.

Встраивание — через существующий механизм оверлеев экрана/приложения (как confirm или `P`); пока оверлей открыт, клавиши экрана не срабатывают, поллинг экрана продолжается.

- **MR detail:** `L` → `Collect` по текущему `s.mr` → оверлей или тост `NoLinks`.
- **Таб MRs:** `L` на строке MR → `tea.Cmd` с `MergeRequests().Get` (msg `mrLinksMsg{Gen, Ref, IID, MR, Err}`). Ответ принимается, если `Gen` текущий и выделен тот же MR; иначе выбрасывается. Успех → `Collect` по деталям; ошибка → тост ошибки и, если `Collect` по `MRRow` не пуст, оверлей с ним. Пусто → тост `NoLinks`. В workspace `L` на заголовке группы и «ещё N» не делает ничего и в футере не показывается.

### 7. Правки SPEC (до кода)
Сделаны в коммите «SPEC: ссылки MR (#51)»: §3.1 (хоткей `L`, разрешённый конфликт с метками issue), §3.4 (Title, действия), §3.5 (блок заголовка, Overview, раздел «Ссылки MR», действия), §4 (`Link`, `TitleLinks`, `DescriptionLinks`), §5.2 (`titleHtml descriptionHtml` в `MRDetail`), §6.1 (`internal/mrlinks`), §7 (`links`), §13.1 п.4.

## Decisions (T23, теги)

### 8. API тегов — REST, интерфейс расширяется
```go
type TagService interface {
    // Tags newest version first: REST order_by=version&sort=desc, Release mapped.
    List(ctx context.Context, p ProjectRef, search, cursor string, limit int) (Page[Tag], error)
    // Pipelines of tags newest first: REST GET /pipelines?scope=tags&per_page=limit&order_by=id&sort=desc.
    Pipelines(ctx context.Context, p ProjectRef, limit int) ([]PipelineSummary, error)
    Create(ctx context.Context, p ProjectRef, name, ref, message string) (Tag, error)
    Delete(ctx context.Context, p ProjectRef, name string) error
    // Compare is REST GET /repository/compare?from&to.
    Compare(ctx context.Context, p ProjectRef, from, to string) (Comparison, error)
}

type Comparison struct {
    Head    *CommitSummary  // последний коммит to (поле commit ответа), nil — нет
    Commits []CommitSummary // коммиты from..to, новые первыми
}

// gitlab.MatchTagPipelines sets Tag.Pipeline: the first pipeline (newest) whose Ref equals the tag name.
func MatchTagPipelines(tags []Tag, pipes []PipelineSummary)

// WorkspaceService:
Tags(ctx context.Context, ps []ProjectRef, search string, perProject int) <-chan BatchResult[Tag]
```
- `List` всегда сортирует `version desc` (минимальная версия GitLab 16.0 это умеет). Порядок подсказок тегов в Run pipeline и фильтре `ref:` меняется с «по имени» на «по версии» — допустимо, SPEC §3.9 говорит «порядок ответа API». `Release` — из `release.tag_name/description` ответа; `Message` — сообщение annotated-тега как есть.
- `Message` в колонке: первая строка сообщения тега, у lightweight-тега (пустое сообщение) — заголовок коммита. Ago — `Commit.CreatedAt`.
- Пайплайны тегов: проект — `Pipelines(ctx, p, 100)`, workspace — `50` (SPEC §5.2); теги старше покрытого окна остаются без пайплайна (`—`). Ответ пайплайнов не пришёл или упал — колонка `—`, ошибка в debug-лог, список тегов показывается.
- `Create`: `message` пустой — lightweight-тег. Ответ 400 «already exists» и прочие — доменная ошибка с сообщением сервера.
- `hybrid.Workspace().Tags`: на проект — `List(search, perProject)` (+ `X-Total` в `Total`) и, если тегов > 0, `Pipelines(50)`; не больше 4 проектов одновременно (свой семафор на вызов); результат — `BatchResult` с одной группой на проект по мере готовности; ошибка проекта — `ProjectGroup.Err`. `rest.Workspace().Tags` — то же (REST-only клиент), `graphql` метода не имеет.
- Фейк: теги в памяти по проекту, `Create/Delete` меняют их, журнал вызовов, инъекция ошибок (403 на protected-тег, 400 на дубль), `Compare` по выдуманным коммитам.
- Кэш (`Env.Cache`, 30 с): `tags:<path>:<search>:<cursor>`, `tagpipes:<path>`, `compare:<path>:<from>:<to>`. Создание и удаление тега инвалидируют `tags:<path>:`, `tagpipes:<path>`, `compare:<path>:` и кэш последнего тега обзора проекта (`hybrid` — ключ `latestTag`).

### 9. `internal/semver` — предложение следующей версии
```go
type Version struct {
    Prefix              string // "v" или ""
    Major, Minor, Patch int
    Pre                 string // "rc2" в v1.14.0-rc2, "" — релиз
}
func Parse(s string) (Version, bool) // ^(v?)(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$
func (v Version) String() string
func Compare(a, b Version) int        // приоритет SemVer 2.0: prerelease < релиз, идентификаторы pre по правилам SemVer

type Kind string // "patch" | "minor" | "major" | "release"
type Option struct {
    Kind    Kind
    Name    string
    Release bool // версия совпала с релизом prerelease-тега (подпись «minor · release»)
}
type Suggestion struct {
    Base    string   // тег-база ("" — нет релизных semver-тегов)
    Options []Option // patch, minor, major, затем отдельный release (если не совпал ни с одним)
    Default int      // индекс варианта по умолчанию
}
func Suggest(names []string) Suggestion
```
- Мусор (`latest`, `release-2023`, `v1.2`) пропускается. База — максимальный релизный (без `Pre`) тег; prefix варианта — prefix базы.
- Patch/minor/major — от базы (`v1.14.2` → `v1.14.3`, `v1.15.0`, `v2.0.0`); по умолчанию patch.
- Максимальный тег вообще — prerelease с ядром выше базы → его релиз (`v1.14.0-rc2` → `v1.14.0`, prefix этого тега) предлагается и становится **вариантом по умолчанию** (после rc обычно выпускают релиз): совпал с patch/minor/major — этот вариант помечается `Release` (подпись `minor · release`); не совпал (`v1.16.0-rc1` при базе `v1.14.2`) — добавляется четвёртым вариантом `release`. Уточнение SPEC §3.10/§11.2: там по умолчанию patch и «добавляется вариант» — правится в коммите SPEC (решение 14).
- Релизных тегов нет: база `0.0.0`, prefix — у максимального prerelease-тега или `v`; варианты `v0.0.1`, `v0.1.0`, `v1.0.0`, по умолчанию minor (`v0.1.0`, SPEC §11.2); prerelease есть — правило release выше (оно важнее).
- Источник имён: форма New tag запрашивает `List(p, "", "", 100)` при открытии; превью таба — загруженные строки (в проекте — без активного поиска, в workspace — строки группы).

### 10. Таб Tags проекта (`project/tags.go`)
- Шаблон — таб Pipelines: страница 20, подгрузка за 5 строк до конца, поллинг 30 с, `r` мимо кэша, курсор держится по имени тега при обновлении. Загрузка страницы — `List` и (только для первой страницы и при обновлении) `Pipelines(100)`, потом `MatchTagPipelines` по всем загруженным.
- Колонки: Tag, Commit (short SHA), Message (+ `◆ release`), Pipeline (глиф статуса и `#id`, нет — `—`), Ago.
- Строка фильтров: слева `/ поиск: <текст>` (без текста — `/ поиск`), справа `сорт: semver ↓`. `/` открывает строку ввода только для свободного текста (без ключей и подсказок); `enter` применяет (запрос с `search`, сброс к первой странице), `esc` — отмена правки. `backspace` вне строки сбрасывает поиск. `m` и `s` в табе ничего не делают.
- Счётчик таба — `Total` (`X-Total`), если известен. Статус футера — `<курсор> / <всего>` (как в макете `6 / 58`).
- Пусто: «Нет тегов», с поиском — «Нет тегов по запросу «v9» · backspace — сбросить поиск».
- Превью (как `previewShown` у Pipelines), общий рендер с workspace — `tagactions.Preview`:
  1. `<тег> @ <short SHA> · <заголовок коммита>` и ` · protected`, если защищён;
  2. compare с default-веткой: `main опережает на 2 коммита` / `main совпадает с тегом` (0 коммитов) / `◐ сравниваем…` / ошибка — `сравнение недоступно` Muted (без тоста). Плюс ` · n предложит v1.14.3` — вариант по умолчанию `Suggest` по именам загруженных строк (в проекте — только без активного поиска). Default-ветка неизвестна — compare не делается, строка только с «n предложит»;
  3. `Release notes: <первая непустая строка описания релиза>`, если есть релиз.
  Compare — лениво по выделенной строке, `tea.Cmd` с `Gen` и ключом тега; ответ для другого тега выбрасывается (как стейджи превью Pipelines).
- Действия: `enter` — экран Pipeline пайплайна тега, нет пайплайна — тост «У тега нет пайплайна»; `n` — форма New tag; `D` — удаление (решение 12); `y` — копировать имя тега (`Copied(What.TagName)`); `o` — `<проект>/-/tags`, с поиском — `?search=<текст>`. Архивный проект — `n` и `D` скрыты и не действуют.
- `project.Options.Tab = TabTags` и `Options.TagSearch` — для `f` из workspace.

### 11. Форма New tag (`screens/newtag`, `app.Overlay`)
Модальная форма по шаблону `runpipeline.Form`. Блоки сверху вниз, как в макете `Tags` (правая панель макета — модалка, как Run pipeline):
- заголовок `Новый тег · <путь проекта>`;
- **Имя**: поле со значением выбранного варианта; под ним `предложено: patch от v1.14.2` (вариант по умолчанию) (без базы — `нет semver-тегов · предложено v0.1.0`) и строка вариантов `patch v1.14.3 · minor v1.15.0 · major v2.0.0 · своё…` (с prerelease — `minor v1.14.0 · release` или отдельный `release v1.16.0`), текущий — Accent. `↑/↓` в фокусе имени меняют вариант. «своё…» делает поле редактируемым (`textinput`), начальное значение — вариант по умолчанию; `↑/↓` и тут переключают варианты. Пока список тегов формы не пришёл — `◐ загружаем теги…`, варианты появляются после ответа; ошибка — тост и только «своё…»;
- **Из ref**: `textinput`, по умолчанию default-ветка проекта; правка — поиск веток `Projects().Branches(search)` через 300 мс, до 5 подсказок под полем, пока поле в фокусе и есть подсказки: `↑/↓` выбор, `enter` подставить. Подпись: `main @ 0b7d3e9 · 2 коммита после v1.14.2` по `Compare(from=<последний тег>, to=<ref>)` (`Head` — SHA, `len(Commits)`), запрос через 300 мс после смены ref; нет тегов — `main · первый тег проекта` (compare не делается); ошибка compare — `ref не найден или сравнение недоступно` Muted;
- **Сообщение (annotated)**: `textinput`, по умолчанию `release: <имя>`; пока пользователь его не правил, оно следует за именем. Пустое — lightweight-тег;
- **В теге окажутся**: до 10 коммитов из того же compare, `<short SHA> <заголовок>`, новые сверху, больше 10 — `… ещё N`; 0 — «нет новых коммитов после v1.14.2»;
- низ: `esc отмена` и кнопка `enter Создать`.

Фокус — по кругу `tab`/`shift+tab`: имя → ref → сообщение → кнопка. `enter` в имени и сообщении — следующее поле, в ref — подставить подсказку (если список виден), иначе следующее поле; создание — только `enter` на кнопке (кнопка в фокусе — активный стиль). `esc` — закрыть без создания.

Кнопка: проверка — пустое имя → «укажи имя тега»; имя совпало с тегом из списка формы → «тег v1.14.2 уже есть»; пустой ref → «укажи ref»; ошибка показывается строкой над кнопкой, фокус уходит в поле с ошибкой. Затем **снимок** `{name, ref, message, head}` и встроенный confirm (как `f.confirm` у Run pipeline, по умолчанию «Отмена»): заголовок `Создать тег v1.14.3 на main @ 0b7d3e9?` (SHA неизвестен — `на main?`), описание `Annotated-тег: «release: v1.14.3»` или `Lightweight-тег без сообщения`. Подтверждение → `Create` со снимком; форма заблокирована, строка хоткеев «создаём…», `esc` не действует. Успех — форма закрывается, тост `TagCreated(name)`, `tagactions.CreatedMsg{Ref, Tag}` для таба: инвалидация кэша, перезагрузка, курсор на новом теге. Ошибка — тост (`Env.Fail`, у 400 — сообщение сервера), форма разблокирована с теми же значениями. Все ответы формы несут `common.Gen` экземпляра; ответ после закрытия выбрасывается.

### 12. Удаление и общие действия (`screens/tagactions`)
По шаблону `pipeactions`:
- `NewTag(env, ref, defaultBranch) tea.Cmd` — открыть форму;
- `Delete(env, ref, tag) tea.Cmd` — confirm `Удалить тег v1.14.2?`, описание `Тег удалится в GitLab, коммит 9fa2c51 останется.`, у защищённого — плюс `Тег защищён: GitLab, скорее всего, откажет.`; по умолчанию «Отмена» → `DeletedMsg{Ref, Name, Protected, Err}`;
- `Done(env, msg)` — инвалидация кэша (решение 8) и тост: успех — `Тег v1.14.2 удалён`; `ErrForbidden` у защищённого — `Тег v1.14.2 защищён`; иначе `Env.Fail`. Таб после успеха перезагружает список (проект — первую страницу, workspace — группу проекта);
- `Preview(...)` — рендер превью (решение 10), общий для проекта и workspace.

### 13. Групповой таб Tags workspace (`workspace/tags.go`)
`groupBase[gitlab.Tag]` с именем таба `tags` — `expanded.tags`/`selected.tags` в `state.json` без изменения формата.
- **Свёрнуты по умолчанию**, как MRs/Pipelines. Свёрнутый заголовок — из обзора: `▸ <путь>  v1.14.2` (`LastTag`), `TagKnown` с пустым тегом — `нет тегов`, иначе только путь. Раскрытый — `▾ <путь>  58 тегов · показано 3` (`Plural.Tags(Total)`), без записей — строка `нет тегов`.
- Запросы — `Workspace().Tags(ctx, refs, search, per_project)`; первое открытие, поллинг 60 с и `r` — только раскрытые группы (в режиме фильтра — все показанные); быстрого тика нет. Заголовки свёрнутых обновляются обзором, как в других табах.
- Строка фильтров: `P` (чип), `я: не применяется к тегам` (всегда, `m` ничего не делает), `/ поиск`; справа `на проект: 3 · сорт: version ↓` и, как в других табах, `показаны …`/`найдено в …`. `/` — свободный текст; непустой текст — **режим фильтра** (все проекты, прошедшие `P`). `backspace` — сброс текста и `P`. `s` ничего не делает.
- Колонки и превью — как в проекте (решение 10); compare — с default-веткой из обзора, «n предложит» — по строкам группы.
- Действия: `enter`, `D`, `y` — на строке тега; `n` — на любой строке группы (запись, заголовок, «ещё N») в проекте группы, как `n` в Pipelines; `o` — страница тегов проекта (с поиском); `f` — экран проекта на табе Tags с текстом поиска. Архивный проект — `n`, `D` скрыты.
- Счётчик таба — только в режиме фильтра (число найденных), как у Pipelines.

### 14. Правки SPEC (до кода)
Коммит «SPEC: теги, T23 в change m3-tags-issues-polish (#23)»: §3.1 (Tags: `/`, `y`, `o`), §3.3 (Tags свёрнуты, заголовки, фильтры, действия, `f` → Tags), §3.4 (таб Tags: Message, превью, поиск, пусто), §3.10 (форма целиком: варианты, фокус, кнопка, проверка, confirm, блокировка), §4 (`Comparison`), §5.2 (порядок `version`, `per_page` пайплайнов тегов), §5.8 (`expanded.tags`, `selected.tags`), §6.1 (`screens/tagactions`, `screens/newtag`), §6.3 (`TagService`, `WorkspaceService.Tags`, `MatchTagPipelines`), §11.2 (нет semver-тегов: варианты, по умолчанию minor; prerelease: релиз — вариант по умолчанию).

## Decisions (T24, issues и комментарии)

### 15. API issues — REST
```go
type RelatedMR struct {
    IID    int64
    Title  string
    State  MRState
    WebURL string
}

// Issue получает поля (остальные — как в SPEC §4):
//   MergeRequestsCount int         // merge_requests_count из списка: есть ли связанные MR
//   RelatedMRs         []RelatedMR // вместо []int64; заполняются отдельным запросом

type IssueFilter struct {
    State     string   // opened (по умолчанию) | closed | all
    Assignee  string   // "" any | "@me" | "none" | username
    Milestone string   // "" any | "none" | title
    Labels    []string // все сразу (AND)
    Search    string
}

type IssueService interface {
    List(ctx context.Context, p ProjectRef, f IssueFilter, cursor string, limit int) (Page[Issue], error)
    Get(ctx context.Context, p ProjectRef, iid int64) (Issue, error)
    // Notes: последние last заметок (system тоже), старые первыми: sort=desc&per_page=last, затем разворот.
    Notes(ctx context.Context, p ProjectRef, iid int64, last int) ([]Note, error)
    // Discussions: все треды, все страницы (per_page=100), system тоже — фильтрует UI.
    Discussions(ctx context.Context, p ProjectRef, iid int64) ([]Discussion, error)
    RelatedMRs(ctx context.Context, p ProjectRef, iid int64) ([]RelatedMR, error)
    Comment(ctx context.Context, p ProjectRef, iid int64, body string) (Note, error)
    SetState(ctx context.Context, p ProjectRef, iid int64, closed bool) (Issue, error) // state_event=close|reopen
}

// MergeRequestService: + Comment(ctx, p, iid, body) (Note, error) — POST /merge_requests/:iid/notes.
// ProjectService:     + Milestones(ctx, p, search) ([]string, error) — GET /projects/:id/milestones?state=active&search&per_page=20 (только проекта).
```
- Фильтр → REST `GET /projects/:id/issues`: `state` (`all` — параметр не пишется), `Assignee`: `@me` → `scope=assigned_to_me` (логин не нужен), `none` → `assignee_id=None`, логин → `assignee_username`; `Milestone`: `none` → `milestone=None`, title → `milestone`; `labels=a,b`; `search`; `order_by=updated_at&sort=desc`; `X-Total` → `Total`.
- `Notes(last)`: в превью `last = 50`; system-заметки отбрасывает UI, поэтому комментариев может быть меньше 50 — допустимо.
- `Client.Issues()` в `rest`, `hybrid` делегирует в REST; GraphQL не трогается.
- Фейк: issues в памяти по проекту с выдуманными данными (`#1318 Surge coefficient ignores zone overrides`, `anna.k`, `egor`, `!479`), заметки и треды, связанные MR, `Comment`/`SetState` меняют данные, журнал вызовов, инъекция ошибок.
- Кэш (`Env.Cache`): `issues:<path>:<фильтр>:<cursor>` 30 с, `issue:<path>:<iid>` 30 с, `inotes:<path>:<iid>` 30 с, `idiscs:<path>:<iid>` 30 с, `irelated:<path>:<iid>` 5 мин. `Comment` и `SetState` сбрасывают `issues:<path>:` и ключи этого issue; комментарий к MR — ключ discussions этого MR.

### 16. Таб Issues проекта (`project/issues.go`)
Шаблон — таб MRs (страницы 20, подгрузка, курсор по IID, поллинг 30 с, `r` мимо кэша, ленивая загрузка при первом открытии).
- **Колонки:** IID (`#1318`), Title (+ ` → !479` Muted, если известен связанный MR; несколько — ` → !479 +1`), Labels (через пробел, обрезка `…`), Upd (относительное `UpdatedAt`).
- **Связанные MR фоном:** после прихода страницы для issues с `MergeRequestsCount > 0` и без свежего кэша — `RelatedMRs` не больше 4 одновременно (`tea.Cmd` на issue с `Gen` таба; общий семафор таба). Ответ дописывает маркер в строку. Тики поллинга не запрашивают, пока кэш 5 мин жив; `r` — мимо кэша.
- **Строка фильтров** (как у MR): `/ state:opened assignee:any label:any milestone:any` и свободный текст. Ключи: `state` (`opened|closed|all`), `assignee` (`any|@me|none` + участники проекта — `membersSource`), `label` (Multi, метки проекта — `labelsSource`), `milestone` (`any|none` + `Projects().Milestones` с поиском через 300 мс, кэш 5 мин как у прочих подсказок). `s` — `state` по кругу opened → closed → all; `m` — `assignee` any ↔ `@me` (фильтр «я»); `backspace` — сброс к умолчаниям.
- **Счётчик таба** — `Total` текущего фильтра; статус футера — `<позиция> / <всего>` (макет: `5 / 12`).
- **Пусто:** без фильтров — «Нет открытых issues»; с `m` — «Нет открытых issues, назначенных тебе · m — снять фильтр «я»»; с прочими фильтрами — «Нет issues по фильтрам · backspace — сбросить».
- **Раскладка превью:** ширина ≥ 140 — справа (список ~55 %, вертикальный разделитель, как в макете); иначе снизу по правилам `previewShown` таба MRs; не помещается — без превью.
- **Превью** выбранного issue:
  1. title (Bold);
  2. `● open · #1318 · anna.k · assignee: egor · milestone: 2026-Q4` (закрытый — `○ closed`; нет assignee/milestone — часть не пишется; несколько assignee — через запятую);
  3. связанные MR, если загружены: `→ !479 Fix slot race · opened`;
  4. описание: `common.Markdown(common.HTMLToMarkdown(desc))`, не больше половины высоты превью, обрезка — строка `…`; пустое — «нет описания» Muted;
  5. `КОММЕНТАРИИ N` (`UserNotesCount`) и последние не-system заметки из `Notes(last=50)`: снизу вверх, сколько влезает целиком; заметка — `author · ago` и тело markdown не длиннее 6 строк (`…`); пока грузятся — `◐ загрузка…`; ошибка — «комментарии недоступны» Muted, без тоста;
  6. последняя строка — `c — написать комментарий…` Muted.
  Заметки — лениво по выбранной строке (`tea.Cmd` с `Gen` и IID, чужие ответы выбрасываются), кэш 30 с.
- **Действия:** `enter` — экран issue (решение 17); `c` — комментарий (решение 19); `x` — close/reopen (решение 20); `o` — список issues проекта в браузере с фильтрами; `O` — выделенный issue; `y` — ссылка выделенного issue. У архивного проекта `c` и `x` скрыты и не действуют.
- **URL `o`:** `<проект>/-/issues`, умолчания не пишутся, `url.Values.Encode`: `state=closed|all`; `assignee` `@me` → `assignee_username=<я>` (логин неизвестен — не пишется), `none` → `assignee_id=None`, логин → `assignee_username=`; `label_name[]=` на каждую метку; `milestone_title=` (`none` → `None`); `search=`.

### 17. Экран issue (`screens/issuedetail`)
Кладётся на стек по `enter`; `esc` — назад, курсор таба сохраняется.
- **Шапка:** `platform/route-service › Issues › #1318`.
- **Тело — одна прокрутка:** title (Bold); строка статуса как в превью; `Labels: bug · pricing`; связанные MR (`→ !479 Fix slot race · opened`, каждая отдельной строкой); разделитель; описание (markdown); заголовок `Комментарии N`; треды карточками — общий пакет `screens/threads` (решение 18), system-заметки скрыты. Нет тредов — «Комментариев нет · c — написать».
- **Данные:** `Issues().Get` и `Issues().Discussions` (оба лениво при открытии, параллельно), `RelatedMRs`; поллинг 30 с (только на вершине стека), `r` — мимо кэша. Ответы с чужим `Gen` выбрасываются; позиция прокрутки и свёрнутость тредов сохраняются при обновлении.
- **Клавиши:** `j/k`, `↓/↑` — строка; `pgdown`/`pgup`, `ctrl+d`/`ctrl+u`, `space` вне тредов — страница; `g/G`; `J/K` — следующий/предыдущий тред (текущий — толстая рамка Accent, прокрутка к нему); `space` при текущем треде — свернуть/развернуть его; `c` — комментарий; `x` — close/reopen; `o` — issue в браузере; `y` — ссылка issue. Правила сворачивания по умолчанию — как в MR (разрешённые и длинные первые заметки).
- **После комментария** — перезапрос discussions и `Get` (счётчик), прокрутка в конец, текущий тред — последний. **После `x`** — `Get`, строка статуса меняется, тост.

### 18. Общие карточки тредов (`screens/threads`)
Перенос из `mrdetail/threads.go` без изменения вида:
```go
type Model struct { /* discussions (без system), fold map, md-кэш, current, width */ }
func New(env *app.Env) *Model
func (m *Model) SetDiscussions(ds []gitlab.Discussion) // свёрнутость и current — по ID, сохраняются
func (m *Model) Layout(width int) (lines []string, cards []Card)
func (m *Model) Next() / Prev() / ToggleCurrent()
func (m *Model) Current() (Card, bool)
type Card struct { ID string; Start, H int; Link string }
```
`mrdetail` переходит на пакет; вкладка Discussions ведёт себя как раньше. Требование: **все существующие golden и тесты MR detail проходят без изменения снимков**. Ссылка треда (`Link`) считается колбэком от экрана (у MR — `<MR>#note_<id>`, у issue — `<issue>#note_<id>`).

### 19. Форма комментария (`screens/comment`, `app.Overlay`)
```go
type Target struct {
    Project ProjectRef
    Kind    string // "issue" | "mr"
    IID     int64
    Title   string
}
func Open(env *app.Env, t Target) tea.Cmd
type SentMsg struct { Target Target; Note gitlab.Note }
```
- **Вид:** модалка; заголовок `Комментарий · #1318 Surge coefficient ignores zone overrides` (MR — `!479 …`, обрезка `…`); `textarea` (bubbles v2) во всю ширину модалки, высота — от 6 строк до половины экрана; под ним `markdown · tab — к кнопке · alt+enter — отправить` Muted; низ — `esc закрыть` и кнопка `enter Отправить` (в фокусе — активный стиль).
- **Фокус:** `tab`/`shift+tab` — textarea ↔ кнопка. В textarea `enter` — перенос строки; `alt+enter` — отправить сразу (там, где терминал его передаёт). На кнопке `enter` — отправить.
- **Проверка:** текст из одних пробелов — строка «комментарий пуст» над кнопкой, запроса нет.
- **Отправка:** снимок текста; форма заблокирована, «отправляем…», `esc` не действует. Успех — форма закрыта, тост «Комментарий добавлен», `SentMsg` экрану (тот перезапрашивает, решение 15 — кэш), черновик стёрт. Ошибка — тост (`Env.Fail`), форма разблокирована с текстом.
- **Черновик:** `esc` закрывает без отправки и сохраняет текст в памяти сессии (`app.Env`, ключ `<хост>|<проект>|<kind>|<iid>`); повторный `c` на том же объекте восстанавливает его. На диск не пишется (SPEC §13.1 п.17).
- Все ответы несут `common.Gen` экземпляра; ответ после закрытия формы выбрасывается (но тост успеха и `SentMsg` всё равно отправляются — комментарий уже создан; черновик стирается).
- **Где `c`:** таб Issues (выделенный issue), экран issue, таб MRs проекта и групповой таб MRs workspace (строка MR, не заголовок группы и не «ещё N»), MR detail (любая вкладка; после отправки — перезапрос discussions, счётчик вкладки `Discussions N`). Архивный проект — `c` скрыт.

### 20. Close / reopen (`x`)
Общий хелпер (например `screens/issueactions`):
- открытый issue → confirm «Закрыть issue #1318?», описание — title issue, по умолчанию «Отмена» → `SetState(closed=true)`;
- закрытый → `SetState(closed=false)` без confirm (обратимо, SPEC §13.1 п.12);
- `StateMsg{Ref, IID, Closed, Issue, Err}`; успех — тост «Issue #1318 закрыт» / «Issue #1318 открыт снова», инвалидация кэша, перезагрузка списка (в табе с `state:opened` закрытый issue пропадает, курсор — на соседнюю строку) или `Get` на экране issue; ошибка — `Env.Fail` (403 — «Недостаточно прав»).

### 21. Правки SPEC (до кода)
Коммит «SPEC: issues и комментарии, T24 в change m3-tags-issues-polish (#24)»: §2.6 (экран Issue, оверлей Comment), §3.1 (Issues: `/`, `s`, `m`, `backspace`, `O`, `y`; MR: `c` без пометки M3), §3.4 (таб Issues целиком), §3.5 (`c` в MR detail), новые §3.15 «Issue» и §3.16 «Комментарий», §4 (`Issue.MergeRequestsCount`, `RelatedMR`), §5.2 (эндпоинты issues, milestones, MR note), §6.1 (`issuedetail`, `comment`, `threads`, `issueactions`), §6.3 (`IssueFilter`, `IssueService`, `MergeRequestService.Comment`, `ProjectService.Milestones`).

## Decisions (исправления по ревью PR #52)

### 22. Владелец действия в сообщении результата
`tagactions.DeletedMsg`, `newtag.CreatedMsg` (= `tagactions.CreatedMsg`) и `issueactions.StateMsg` получают поле `Owner common.Token` — токен `Gen` экрана/таба, который запустил действие. Кто запускает — передаёт свой токен: `tagactions.Delete(env, owner, ref, tag)`, `tagactions.NewTag(env, owner, ref, branch)` → `newtag.New(..., owner)`, `issueactions` (close/reopen) — так же. Проверка — `gen.Owns(msg.Owner)` (тот же экземпляр счётчика, поколение не важно: действие уже случилось, его результат выбрасывать нельзя).

Приёмники широковещательного сообщения:
- **владелец** — `Done`/`Created` (тост или `Env.Fail`, инвалидация кэша), локальная правка списка (решение 23), перезапрос;
- **не владелец того же проекта** — без тоста и без `Env.Fail`, при успехе только локальная правка своих данных (список, `forgetLastTag`/`setLastTag` в workspace) и перезагрузка: сразу, если активен, иначе — при возвращении (флаг `dirty` или эквивалент; кэш уже сброшен владельцем); при ошибке — ничего;
- чужой проект — ничего.

Так закрывается двойная обработка удаления/создания тега, когда проект открыт из workspace (`f`): тост и запросы идут один раз. Таб Issues и экран issue переходят с проверки `active` на проверку владельца; поведение «экран issue сверху показал тост, таб перезагрузится при возвращении» сохраняется.

### 23. Обновление списка после действия не оставляет устаревший хвост
Обновление при загруженных страницах склеивает свежую первую страницу и старый хвост (как в табе MRs с M1). Если действие убрало строку из хвоста, хвост её возвращал. Решение — до перезапроса править список локально по результату действия:
- **тег удалён** (`DeletedMsg` без ошибки) — таб Tags проекта убирает тег по имени из `items` (`firstLen`, если строка была в первой странице, и `total` при `total ≥ 0` уменьшаются на 1), курсор — на соседнюю строку; затем `loadFirst(false, true)`. Групповой таб workspace — то же для группы проекта.
- **issue закрыт/переоткрыт** (`StateMsg` без ошибки) — таб Issues заменяет строку по IID на `msg.Issue`; если новое состояние не проходит фильтр `state` (`opened` → закрыт, `closed` → переоткрыт), строка убирается (`firstLen`/`total` как выше, курсор — на соседнюю); затем перезапрос.
- В табе Issues ветка `refresh && loaded && len(items) > firstLen` больше не перезаписывает `t.next` — курсор подгрузки остаётся за уже загруженным хвостом (как в табах Tags и MRs). Иначе уже загруженные страницы запрашиваются повторно.

Подгрузка продолжается с `t.next` старой страницы: после удаления строки сервер сдвинул выборку на одну позицию, и одна строка на стыке страниц не попадёт в список до сброса (offset-пагинация; цена того, что загруженные страницы не запрашиваются повторно).

Строки, пропавшие на сервере без нашего действия, остаются в хвосте до сброса списка (`r` со сбросом, смена фильтра) — как и в табе MRs; это в Risks.

### 24. Поллинг не сбрасывает сравнение тега
`Comparer` получает `Expire(force bool)`: отменяет запрос в полёте, сдвигает поколение, готовые записи помечает устаревшими (`Stale`, число коммитов остаётся), `Loading`/`Failed` забывает. `Load` для устаревшей записи запрашивает заново, но запись до ответа показывается со старым значением — в превью нет мигания «сравниваем…». `force` из `Expire` относится к следующему запросу: поллинг — `false` (через кэш, `CompareTTL`), `r` — `true` (мимо кэша). Ошибка повторного запроса — `Failed`, как сейчас.

Поллинг и `r` в табе Tags проекта (`loadFirst(false, true)`) и `Refresh()` группового таба вызывают `Expire`, а не `Reset`. `Reset` остаётся там, где он сейчас при `reset`-загрузке (смена поиска/фильтра) и при создании/удалении тега.

### 25. Превью issue не рендерит markdown на каждый `View`
`issueactions.Preview` рендерит описание и тела заметок через glamour при каждом вызове. Вводится кэш рендера (по образцу кэша в `issuedetail`/`threads`): ключ — текст (после `HTMLToMarkdown`) и ширина; кэш живёт в табе Issues (передаётся в `Preview` через `PreviewData` или аргумент), сбрасывается на `ThemeChangedMsg`/`LangChangedMsg`; размер ограничен (например, записи только для текущего выбранного issue — при смене IID кэш очищается). Повторный `View` без изменений не вызывает glamour.

### 26. Связанные MR на экране issue — всегда
`merge_requests_count` в REST считает только MR, которые закроют issue при мерже; MR с «Related to #12» в нём не учитываются. Экран issue запрашивает `related_merge_requests` всегда (один запрос, кэш 5 мин), без проверки `MergeRequestsCount`. В табе Issues фильтр `merge_requests_count > 0` остаётся — экономия запросов; маркер `→ !479` в списке показывается только для закрывающих MR (в Risks). Правка SPEC §3.4 (оговорка про счётчик), §3.15 («связанные MR — всегда, отдельным запросом»), §5.2 (строка «Issue (экран)» — без условия) и delta spec `issues/spec.md`.

### 27. `X-Total: 0` — это ноль
REST-обёртки ставят `Total` только при `TotalItems > 0`, поэтому пустой результат выглядит как «неизвестно» (`-1`). `Total` = `TotalItems`, если заголовок `X-Total` присутствует в ответе (`resp.Header.Get("X-Total") != ""`), иначе `-1` (GitLab не отдаёт его для больших выборок). Места: `rest/issues.go` (список issues), `rest/refs.go` (список тегов; теги workspace берут `Total` оттуда же, `rest/workspace.go` не менялся). Общий хелпер `totalOf`. Полоса табов по-прежнему прячет счётчик `0` (`Count > 0`, правило M1): `0` виден через `Count()` и статус, не в заголовке таба.

### 28. Список коммитов формы New tag не выше `avail`
`commitsView` при `room == 1` и нескольких коммитах выводил подпись, коммит и «… ещё N» — 3 строки при `avail = 2`. Инвариант: число строк результата ≤ `avail` при любых `avail` и числе коммитов. При `room == 1`: один коммит — подпись и коммит; больше одного — подпись и строка «… ещё N» с полным числом (`CommitsMore(len(cs))`), без коммитов.

### 29. `c` в MR архивного проекта
SPEC §11.2 (архивный проект: действия записи скрыты). `mrdetail.New` получает `archived bool` (как `issuedetail.New`): таб MRs проекта передаёт `t.sh.archived`, групповой таб MRs workspace — `Archived` из обзора проекта строки (хелпер как `tagsTab.archived`). В MR detail и в групповом табе MRs workspace у архивного проекта `c` не показывается в хелпе и футере и не действует. Прочие действия MR (`a`, `M`, `b`, `C`) — как сейчас, вне этого исправления.

## Decisions (T25, полировка)

### 30. 80×24 и узкий терминал — инвариант на все экраны
Общий тест (по пакетам экранов, на фейке): каждый экран и оверлей в типичном состоянии с данными (списки — с длинными заголовками; ru-строки длиннее en) рендерится на 80×24 и на 120×40, en и ru; у каждой строки `ansi.StringWidth ≤ width`, строк `≤ height`. Перечень: старт/setup, табы проекта (MRs, Pipelines, Issues, Tags), MR detail (все вкладки), Pipeline, Job log, экран issue, workspace (все табы, свёрнуто и развёрнуто), поиск и picker, help `?`, оверлеи (confirm, New tag, Comment, Run pipeline, merge, ссылки `L`). Ширина < 80 или высота < 24 — сообщение «расширь терминал до 80×24» (SPEC §11.2), тоже проверяется.

Найденное переполнение исправляется в экране (ширина колонки, `render.Clip`, приоритеты футера); golden такого экрана может измениться — это указывается в коммите. Недостающие golden 80×24 добавляются (как минимум Tags workspace и help). Существующие снимки без найденной проблемы не меняются.

### 31. Аудит ошибок по SPEC §11.1
Сверка таблицы §11.1 и пунктов про 401/офлайн с кодом (`gitlab/httpx`, `gitlab/errors.go`, `rest`, `graphql`, `ui/app/session.go`, `Env.Fail`). На каждую строку — тест:
- **маппинг** — httptest для REST и GraphQL: 401, 403, 403 с `insufficient_scope`, 404, `project: null`, 409, 405/422, 429 с `Retry-After` и без, DNS-ошибка, TLS-ошибка (`httptest.NewTLSServer` без CA), таймаут → нужный `Err*`;
- **тост и подсказка** — `Env.Fail` на каждый `Err*`: текст en/ru, подсказка 401 по источнику токена (config / env / glab с `--hostname`), 403 scope, TLS → `tls.ca_file`, 429 — «повтор через N с» из `Retry-After`;
- **401 посреди сессии** — поллинг всех экранов стека останавливается (журнал фейка: тики не дают запросов), в шапке `✗ auth`, `r` проверяет токен, после успеха поллинг возобновляется;
- **офлайн** — данные из кэша, шапка `⟳ 5m (офлайн)`, задержка поллинга растёт экспоненциально до 60 с (фейковые часы), после восстановления — тост «соединение восстановлено» один раз;
- **тосты** — 4 с / Error 8 с, один одновременно, новый вытесняет старый, последний остаётся в футере.

Расхождение кода со SPEC — исправить код. Если исправление большое (новая подсистема) — не делать в T25: завести issue (веха по смыслу, обычно `После MVP`, метка `feature`, ссылка в #36). Если код сознательно расходится со SPEC и это лучше — правка SPEC с пояснением.

### 32. Тема `terminal-faint` (SPEC §13.2, вопрос 1)
Muted = ANSI 8 в Solarized Dark совпадает с фоном (base03). Решение — вторая встроенная тема, по выбору в конфиге (`theme: terminal-faint`); по умолчанию остаётся `terminal`:
- `Theme` получает флаг `Faint bool`; `NewStyles` при `Faint` строит Muted-стили и производные (`HeaderMuted`, `ColumnHeader`, `RowMuted`, `KeyDesc`, `Status` created/skipped/canceled и т.п.) как цвет терминала по умолчанию + атрибут Faint, без ANSI 8;
- Border в `terminal-faint` на тёмном фоне — ANSI `0` (в Solarized это base02, отличим от фона), на светлом — как в `terminal`;
- markdown (glamour `StyleConfig` из `Theme`): код и цитаты — Faint вместо цвета Muted;
- `NO_COLOR`: Faint — атрибут, не цвет, остаётся (как Bold и reverse);
- `config`: допустимые значения `theme` — `terminal`, `terminal-faint`; неизвестное — предупреждение-тост и `terminal` (как прочие проверки конфига); `cmd` выбирает конструктор; `--debug-styles` показывает выбранную тему.

Golden-снимки в профиле Ascii от темы не зависят и не меняются. В §13.2 вопрос 1 — «решено: тема `terminal-faint`», §7 и §9.2 описывают тему. Какая тема лучше на реальных палитрах — ручная проверка по чек-листу #25.

### 33. README
README дополняется по текущему коду: workspace (папка с проектами, группа `F`, табы, `u`/`U`), поиск и picker, табы Tags и Issues, форма New tag, комментарии, `x`, ссылки MR (`L`, раздел `links` конфига с примером), темы (`terminal`, `terminal-faint`, `NO_COLOR`), хоткеи M2–M3 (сверить с SPEC §3.1 и help `?`). Только выдуманные хосты и проекты (`gitlab.example.com`, `platform/route-service`). Раздел «Статус»: M0–M3 реализованы, ручная проверка — #25.

### 34. Правки SPEC (до кода)
Коммит «SPEC: полировка, T25 в change m3-tags-issues-polish (#25)»: §7 (значения `theme`), §9.2 (тема `terminal-faint`), §12.3 (инвариант 80×24 в автотестах; ручной чек-лист — в #25), §13.2 (вопрос 1 решён). Правки по итогам аудита (решение 31) — отдельным коммитом SPEC, если понадобятся.

## Risks / Trade-offs

- **Ложные вхождения в описании:** текст ссылки может встретиться в блоке кода (GitLab там ссылку не ставит), и наложение сделает его кликабельным. Принято: клик ведёт туда же, куда и ссылка GitLab с тем же текстом.
- **Перенос строки разрывает ссылку** (`ABC-` / `123`): не кликабельна, но есть в `L`.
- **Старые инстансы без `titleHtml`/`descriptionHtml`:** `#@opt` выкидывает поля, остаются правила и голые URL — без ошибки.
- **Лишний запрос по `L` в списке:** один `MRDetail` по нажатию; кэш деталей делает повторные нажатия дешёвыми.
- **tmux режет OSC 8** без `terminal-features hyperlinks` — для этого и есть `L`.
- **T23: пайплайны старых тегов не видны** — окно 100 (проект) / 50 (workspace) последних tag-пайплайнов. Принято: старые теги редко нужны с пайплайном; `o` ведёт в веб.
- **T23: `order_by=version` против semver клиента** — GitLab сортирует по своей версии (`VersionSorter`), `semver.Compare` может расходиться на prerelease. Список показывается в порядке сервера, предложение версии считает клиент по 100 последним — расхождение влияет только на порядок строк.
- **T24: фоновые `related_merge_requests`** — до 20 запросов на страницу в худшем случае. Ограничено: только issues с `merge_requests_count > 0`, ≤ 4 одновременно, кэш 5 мин, тики не перезапрашивают.
- **T24: `alt+enter`** не приходит в части терминалов (macOS Terminal без «Option as Meta») — основной путь `tab` → кнопка.
- **T24: перенос карточек тредов** может незаметно поменять вид MR detail — страхуют существующие golden-снимки, менять их запрещено.
- **T24: milestones групп** не подсказываются (только проекта): `include_ancestors` есть не во всех поддерживаемых версиях; ввод своего title работает.
- **Ревью #52: хвост списка после поллинга** — строки, удалённые на сервере не из TUI, остаются в загруженных страницах до сброса списка (как у MRs с M1). Свои действия (удаление тега, close/reopen) правят список локально (решение 23).
- **Ревью #52: маркер связанных MR в списке issues** — только для MR, закрывающих issue (`merge_requests_count`); упоминающие MR видны на экране issue (решение 26).
- **T25: Faint не везде одинаков** — часть терминалов рисует Faint как обычный цвет или слишком тускло; поэтому `terminal-faint` — по выбору, а не по умолчанию.
- **T25: аудит ошибок может найти больше, чем помещается в веху** — крупное заводится отдельными issues (решение 31).
- **T23: compare больших диапазонов** — REST compare отдаёт до сотен коммитов и может быть медленным; запрос ленивый, только по выбранной строке, с кэшем 30 с.

## Migration Plan

Новое поле конфига необязательное, без правил поведение как раньше плюс ссылки GitLab в MR detail. Миграции данных нет.

T24: миграций нет; черновики комментариев — только в памяти.

T23: `state.json` получает ключи `tags` в `expanded`/`selected` — формат прежний, старые файлы читаются как «всё свёрнуто».

## Open Questions

- нет
