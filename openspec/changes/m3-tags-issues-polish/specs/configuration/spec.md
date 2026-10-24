## ADDED Requirements

### Requirement: Правила ссылок `links`
Конфиг SHALL поддерживать список `links: [{pattern, url}]`: `pattern` — регулярное выражение Go (RE2), `url` — шаблон `regexp.Expand` (`$0`, `$1`, `${name}`). Правило с пустым `pattern` или `url`, с ошибкой в регулярке или с `url`, не начинающимся с `http://`/`https://`, SHALL пропускаться с тостом-предупреждением, называющим ключ (`links[0].pattern`); остальные правила работают, запуск не прерывается. По умолчанию правил нет.

#### Scenario: Битая регулярка
- **WHEN** в конфиге `links: [{pattern: '[A-Z+', url: 'https://tracker.example.com/browse/$0'}, {pattern: '[A-Z]+-\d+', url: 'https://tracker.example.com/browse/$0'}]`
- **THEN** при старте тост-предупреждение с `links[0].pattern`, второе правило работает

#### Scenario: Группы в шаблоне
- **WHEN** правило `pattern: 'INC(\d+)'`, `url: 'https://tracker.example.com/incident/$1'`, title `INC42 hotfix`
- **THEN** ссылка `INC42 — https://tracker.example.com/incident/42`

### Requirement: Значение `theme`
Конфиг SHALL принимать `theme: terminal` (по умолчанию) и `theme: terminal-faint`. Неизвестное значение SHALL заменяться на `terminal` с тостом-предупреждением, называющим ключ `theme`; запуск не прерывается.

#### Scenario: Тема terminal-faint
- **WHEN** в конфиге `theme: terminal-faint`
- **THEN** второстепенный текст рисуется цветом по умолчанию с атрибутом Faint, без ANSI 8

#### Scenario: Неизвестная тема
- **WHEN** в конфиге `theme: foo`
- **THEN** при старте тост-предупреждение с `theme`, используется `terminal`
