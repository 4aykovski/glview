## ADDED Requirements

### Requirement: `d` в групповом табе MRs
`d` на строке MR группового таба MRs workspace SHALL открывать MR detail этого MR (в контексте проекта строки) на вкладке Diff. На заголовке группы и строке «ещё N» `d` MUST NOT действовать.

#### Scenario: Дифф из workspace
- **WHEN** курсор на строке !479 группы `platform/route-service` и пользователь нажал `d`
- **THEN** открыт MR detail !479 проекта `platform/route-service` на вкладке Diff

#### Scenario: На заголовке группы
- **WHEN** курсор на заголовке группы и пользователь нажал `d`
- **THEN** ничего не происходит
