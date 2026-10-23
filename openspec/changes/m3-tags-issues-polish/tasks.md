## 0. Подготовка

- [x] 0.1 Ветка `m3` от свежего `main`, `git config --get core.hooksPath` = `scripts/githooks` (иначе `make hooks`); проверка: `git branch --show-current` = `m3`
- [x] 0.2 Правки SPEC по design.md, решение 7: §3.1, §3.4, §3.5, §4, §5.2, §6.1, §7, §13.1; проверка: `make confidential` зелёный, коммит «SPEC: ссылки MR (#51)»

## 1. Ссылки MR (#51, T34)

- [x] 1.1 `internal/gitlab`: `Link{Text, URL}`, `MergeRequest.TitleLinks/DescriptionLinks`; проверка: `go build ./...`
- [x] 1.2 `internal/mrlinks`: `FromHTML(html, origin)` (решение 1), `Rule`, правила и голые URL (решение 2), `Collect` (решение 3); проверка: unit-тесты — якорь заголовка и `mailto:` пропущены, `/anna.k` → `https://gitlab.example.com/anna.k`, сущности раскрыты; `$0`/`$1`/`${name}`; хвост `.`, `)` без пары и `),` обрезаны, `(https://x.example.com/a_(b))` цел; порядок title → description по позиции, дубли по URL схлопнуты
- [x] 1.3 `mrlinks.Apply(rendered, links)` (решение 4); проверка: unit-тесты — `xansi.Strip(Apply(s)) == xansi.Strip(s)` на выводе `common.Markdown` с заголовками, списками, кодом и ссылками; `#1` не матчит `#12`; внутрь OSC 8 glamour не вкладывается; совпадение внутри SGR-раскраски и через границу стилей; несколько строк; OSC 8 всегда сбалансированы
- [x] 1.4 `graphql`: `titleHtml`, `descriptionHtml` в `MRDetail` отдельными `#@opt`, маппинг через `FromHTML` с `origin()`; `MRRow` не меняется; проверка: тест на JSON-фикстуре (выдуманные данные) и тест «Field 'titleHtml' doesn't exist» → повтор без поля, ссылки пусты
- [x] 1.5 Фейк: ссылки в деталях MR (`TitleLinks/DescriptionLinks` для тестов и демо); журнал вызовов `Get`; проверка: `go test ./internal/gitlab/...`
- [x] 1.6 `config`: `Links []LinkRule` (`yaml:"links"`), проверка правил с `Warning` по ключу `links[i].pattern`/`links[i].url` (решение 2); `cmd` компилирует правила в `[]mrlinks.Rule` и кладёт в `app.Env`; проверка: unit-тесты конфига — битая регулярка, пустой url, `ftp://` → предупреждения, корректное правило осталось; пример `links` в §7 разбирается
- [x] 1.7 `ui/common`: оверлей списка ссылок (решение 6) на рамке `ListModal`; проверка: unit-тесты — `j/k`/`g/G`, обрезка `…`, `text == url` → одна колонка, `SetLinks` держит выделение по URL
- [x] 1.8 MR detail: OSC 8 в title и описании Overview (решение 5), `L` → оверлей или тост `NoLinks`, `enter` → `Env.Browser`, `y` → `SetClipboard`; хелп и футер; проверка: uitest — сценарии merge-requests/spec.md; существующие golden MR detail без изменений; новый golden оверлея 120×40 и 80×24
- [ ] 1.9 Таб MRs режима проекта: OSC 8 в Title и превью по правилам и голым URL; `L` → `MergeRequests().Get` с `Gen` → оверлей; ошибка → тост + оверлей по `MRRow`; проверка: uitest — сценарии project-view/spec.md (журнал фейка, выброс ответа после смены курсора), OSC 8 в ячейке сбалансированы, golden списка без изменений
- [ ] 1.10 Групповой таб MRs workspace: то же, `L` в контексте проекта строки, на заголовке и «ещё N» не действует; проверка: uitest — сценарий workspace/spec.md, golden без изменений
- [ ] 1.11 i18n en/ru: подпись клавиши `L`, заголовок оверлея, подсказки, тост `NoLinks`, `What.Link` для `Copied`, предупреждение конфига; тест полноты зелёный
- [ ] 1.12 `make test lint confidential` зелёные; коммит(ы) `T34: … (#51)`; отметить сделанное в «Готово, когда» #51; ручная проверка на живом инстансе (ключ трекера в заголовке открывается) — в «Осталось», issue открыт
