## ADDED Requirements

### Requirement: Ссылки MR
Ссылки MR SHALL собираться из трёх источников: `<a href>` из GraphQL `titleHtml`/`descriptionHtml` (только `href` и текст; без пустого текста, якорей `#…` и схем кроме `http(s)`; относительный `href` дополняется origin'ом инстанса), правил `links` конфига по тексту title и description и голых `http(s)://` в title и description. Общий список SHALL идти в порядке текста: сначала title, потом description; дубли по URL схлопываются.

#### Scenario: Ключ трекера из интеграции
- **WHEN** title MR `ABC-123: fix slot race`, а `titleHtml` содержит `<a href="https://tracker.example.com/browse/ABC-123">ABC-123</a>`
- **THEN** первая ссылка списка — `ABC-123 — https://tracker.example.com/browse/ABC-123`

#### Scenario: Правило конфига без интеграции
- **WHEN** в конфиге правило `pattern: '[A-Z]+-\d+'`, `url: 'https://tracker.example.com/browse/$0'`, title `ABC-123: fix`, в `titleHtml` ссылки нет
- **THEN** в списке ссылка `ABC-123 — https://tracker.example.com/browse/ABC-123`

#### Scenario: Дубли и относительные ссылки
- **WHEN** title и description оба ссылаются на `!45`, в `descriptionHtml` есть `<a href="/anna.k">@anna.k</a>` и `<a href="#setup" class="anchor"></a>`
- **THEN** `!45` в списке один раз (из title), `@anna.k` ведёт на `https://gitlab.example.com/anna.k`, якоря заголовка в списке нет

#### Scenario: Инстанс без titleHtml
- **WHEN** GitLab отвечает «Field 'titleHtml' doesn't exist»
- **THEN** детали MR загружаются без этих полей, ссылки — из правил и голых URL, ошибки нет

### Requirement: OSC 8 в title и описании MR detail
В MR detail title блока заголовка и описание на Overview SHALL выводить ссылки OSC 8 (cmd/ctrl+клик открывает в браузере), накладывая их на уже отрисованный текст целыми токенами. Видимый текст MUST NOT меняться, существующие ссылки glamour MUST NOT вкладываться друг в друга.

#### Scenario: Кликабельный ключ в заголовке
- **WHEN** открыт MR с title `ABC-123: fix slot race` и ссылкой на трекер
- **THEN** `ABC-123` в блоке заголовка обёрнут OSC 8 с URL трекера, `xansi.Strip` вида совпадает с видом без ссылок

#### Scenario: Ссылка GitLab в описании
- **WHEN** описание `Closes #12`, в `descriptionHtml` `#12` ссылается на issue
- **THEN** `#12` в Overview обёрнут OSC 8, URL рядом не дописан

#### Scenario: Не часть другого слова
- **WHEN** ссылка с текстом `#1` и в описании встречается `#12`
- **THEN** `#12` не становится ссылкой `#1`

### Requirement: Оверлей ссылок `L` в MR detail
`L` в MR detail SHALL открывать оверлей со списком ссылок MR: строки `текст — URL`, `j/k` ход, `enter` открывает выделенную ссылку в браузере и закрывает оверлей, `y` копирует URL и закрывает, `esc` закрывает. Если ссылок нет, оверлей MUST NOT открываться, показывается тост «В MR нет ссылок».

#### Scenario: Открыть ссылку
- **WHEN** в MR две ссылки, нажаты `L`, `j`, `enter`
- **THEN** в браузере открыт URL второй ссылки, оверлей закрыт

#### Scenario: Скопировать ссылку
- **WHEN** нажаты `L`, `y`
- **THEN** URL первой ссылки в буфере (OSC 52), тост «Скопировано: ссылка»

#### Scenario: Ссылок нет
- **WHEN** у MR без ссылок в title и описании нажат `L`
- **THEN** тост «В MR нет ссылок», оверлея нет
