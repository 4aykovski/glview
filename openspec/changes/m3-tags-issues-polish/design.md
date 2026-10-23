## Context

M0–M2 закрыты (архивы `2026-10-19-m0-foundation`, `2026-10-20-m1-project-mode`, `2026-10-22-m2-workspace`). Change общая на веху M3; в ней #51 (T34, сделана) и #23 (T23, теги). Мотивация — proposal.md, поведение — specs/ этой change, полная картина — docs/SPEC.md §3.5 («Ссылки MR»), §7 — для T34; §3.3, §3.4, §3.10, §6.3, §11.2 — для T23.

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

## Goals / Non-Goals

**Goals (T34):**
- Ключ трекера, `#12`, `!45`, `@user` и URL в title и описании MR открываются cmd/ctrl+кликом в MR detail.
- Тот же набор ссылок доступен с клавиатуры (`L`) там, где OSC 8 не работает.
- Без новых запросов при обычном просмотре списка; без роста сложности `MRRow`.

**Goals (T23):**
- Таб Tags проекта и workspace по SPEC §3.3, §3.4: список, поиск, пайплайны тегов, превью с compare.
- Создание тега формой New tag с предложением версии (`internal/semver`) и confirm; удаление `D` с confirm.
- Нагрузка workspace — как у MRs/Pipelines: запросы только по раскрытым группам.

**Non-Goals (T34):**
- Ссылки в тредах Discussions (там markdown-ссылки glamour, как сейчас).
- Ссылки в title списка MR из HTML GitLab (`#12` и т. п. в списке не кликабельны — только правила и голые URL).
- Разбор ссылок GitLab на клиенте (`#12` → URL issue) — источник истины HTML GitLab: он знает, существует ли объект, и про интеграции трекеров.
- Ссылки в issues (#24) — позже, тем же пакетом.

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

## Risks / Trade-offs

- **Ложные вхождения в описании:** текст ссылки может встретиться в блоке кода (GitLab там ссылку не ставит), и наложение сделает его кликабельным. Принято: клик ведёт туда же, куда и ссылка GitLab с тем же текстом.
- **Перенос строки разрывает ссылку** (`ABC-` / `123`): не кликабельна, но есть в `L`.
- **Старые инстансы без `titleHtml`/`descriptionHtml`:** `#@opt` выкидывает поля, остаются правила и голые URL — без ошибки.
- **Лишний запрос по `L` в списке:** один `MRDetail` по нажатию; кэш деталей делает повторные нажатия дешёвыми.
- **tmux режет OSC 8** без `terminal-features hyperlinks` — для этого и есть `L`.
- **T23: пайплайны старых тегов не видны** — окно 100 (проект) / 50 (workspace) последних tag-пайплайнов. Принято: старые теги редко нужны с пайплайном; `o` ведёт в веб.
- **T23: `order_by=version` против semver клиента** — GitLab сортирует по своей версии (`VersionSorter`), `semver.Compare` может расходиться на prerelease. Список показывается в порядке сервера, предложение версии считает клиент по 100 последним — расхождение влияет только на порядок строк.
- **T23: compare больших диапазонов** — REST compare отдаёт до сотен коммитов и может быть медленным; запрос ленивый, только по выбранной строке, с кэшем 30 с.

## Migration Plan

Новое поле конфига необязательное, без правил поведение как раньше плюс ссылки GitLab в MR detail. Миграции данных нет.

T23: `state.json` получает ключи `tags` в `expanded`/`selected` — формат прежний, старые файлы читаются как «всё свёрнуто».

## Open Questions

- нет
