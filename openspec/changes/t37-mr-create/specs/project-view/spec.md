## ADDED Requirements

### Requirement: `n` — новый MR
`n` в табе MRs режима проекта SHALL открывать форму New MR (merge-requests, «Форма New MR») для проекта; у архивного проекта `n` MUST NOT показываться и действовать. В футере и хелпе — `n новый MR`.

#### Scenario: Новый MR из таба
- **WHEN** пользователь нажал `n` в табе MRs
- **THEN** открыт экран `platform/route-service › MRs › новый MR`; `esc` возвращает к списку с прежним курсором

#### Scenario: Архивный проект
- **WHEN** проект архивный
- **THEN** `n` нет в футере, нажатие ничего не делает
