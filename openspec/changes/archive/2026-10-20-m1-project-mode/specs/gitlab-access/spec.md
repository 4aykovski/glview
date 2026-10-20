## ADDED Requirements

### Requirement: Auto-merge по версии инстанса
При merge «когда пайплайн пройдёт» система SHALL передавать `auto_merge=true` на GitLab ≥ 17.11 и `merge_when_pipeline_succeeds=true` на более старых версиях. Версия инстанса берётся из `GET /version` один раз за сессию.

#### Scenario: Старый инстанс
- **WHEN** версия инстанса 16.11, пользователь выбрал auto-merge
- **THEN** запрос merge содержит `merge_when_pipeline_succeeds=true` и не содержит `auto_merge`

#### Scenario: Новый инстанс
- **WHEN** версия инстанса 17.11.2-ee
- **THEN** запрос merge содержит `auto_merge=true`

### Requirement: Опциональные поля GraphQL
Если инстанс отвечает ошибкой «Field 'x' doesn't exist» на опциональное поле (статистика диффа MR), система SHALL повторить запрос без этого поля, запомнить решение до конца сессии и показывать данные без него. Ошибка SHALL записываться в debug-лог, пользователю не показывается.

#### Scenario: Нет статистики диффа
- **WHEN** инстанс не знает поле статистики диффа
- **THEN** список MR и детали отображаются, превью без строки `+N −M`, повторные запросы сразу идут без поля

### Requirement: Идентификация меня в списках
Признак «я одобрил» и фильтры «я» SHALL вычисляться по имени текущего пользователя из `GET /user`, полученного при старте.

#### Scenario: Одобрил я
- **WHEN** среди одобривших MR есть текущий пользователь
- **THEN** MR помечен как «одобрен мной», `a` снимает одобрение

### Requirement: Перепроверка статуса MR
`MergeRequestService` SHALL предоставлять `RecheckMergeStatus(ctx, project, iid)`, который запрашивает REST `GET /projects/:id/merge_requests/:iid?with_merge_status_recheck=true` и возвращает `detailed_merge_status` из ответа. Это единственное место, где детали MR читаются через REST: обычный `Get` остаётся на GraphQL.

#### Scenario: Запуск перепроверки
- **WHEN** вызван `RecheckMergeStatus` для !479
- **THEN** к инстансу ушёл `GET …/merge_requests/479?with_merge_status_recheck=true`, метод вернул `checking`
