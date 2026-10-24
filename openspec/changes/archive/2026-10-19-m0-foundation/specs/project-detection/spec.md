## Purpose

Определяет GitLab-проект и локальную ветку по файлам `.git` без вызова git, а также находит проекты в подпапках для режима workspace.

## ADDED Requirements

### Requirement: Поиск репозитория вверх по дереву
Система SHALL искать `.git`, начиная с текущей директории и поднимаясь к корню, и использовать первый найденный. Система MUST NOT запускать процесс `git` для определения проекта.

#### Scenario: Запуск во вложенной папке
- **WHEN** glview запущен в `repo/internal/pkg`, а `.git` лежит в `repo`
- **THEN** корнем репозитория считается `repo`

### Requirement: `.git` как файл
Система SHALL поддерживать `.git`-файл со строкой `gitdir: <path>` (относительный путь — от файла). Для worktree (в gitdir есть `commondir`) конфиг SHALL читаться из общего каталога, а `HEAD` — из gitdir worktree. Для сабмодуля SHALL использоваться его собственный gitdir.

#### Scenario: Worktree
- **WHEN** запуск в worktree на ветке `feat/x`, основной репозиторий на `main`
- **THEN** remote берётся из общего конфига, ветка — `feat/x`

#### Scenario: Сабмодуль
- **WHEN** запуск в папке сабмодуля
- **THEN** remote и ветка — сабмодуля, а не родительского репозитория

### Requirement: Выбор remote и ветки
Система SHALL использовать remote `origin`. Если `origin` нет и remote ровно один, SHALL использоваться он. Иначе проект не определён. Ветка SHALL браться из `HEAD`; при detached HEAD — короткий SHA с признаком detached.

#### Scenario: Detached HEAD
- **WHEN** `HEAD` содержит SHA
- **THEN** веткой показывается 8-символьный SHA, признак detached установлен

#### Scenario: Несколько remote без origin
- **WHEN** есть remote `upstream` и `fork`, но нет `origin`
- **THEN** проект не определён

### Requirement: Разбор URL remote
Система SHALL разбирать формы `git@host:path.git`, `ssh://git@host[:port]/path.git`, `https://host[:port][/prefix]/path[.git]` и `http://…`. Путь проекта — путь URL без `.git` и без path-prefix инстанса. Хосты SHALL сравниваться без учёта регистра и порта. Хост также считается своим, если он перечислен в `host_aliases`.

#### Scenario: Relative URL root
- **WHEN** хост инстанса `https://example.com/gitlab`, remote `https://example.com/gitlab/group/sub/proj.git`
- **THEN** путь проекта `group/sub/proj`

#### Scenario: SSH-порт
- **WHEN** remote `ssh://git@gitlab.example.com:2222/team/app.git`, хост инстанса `https://gitlab.example.com`
- **THEN** проект распознан как `team/app` этого инстанса

#### Scenario: SSH-алиас
- **WHEN** remote `git@gitlab-work:team/app.git`, в конфиге `host_aliases: [gitlab-work]`
- **THEN** проект распознан как `team/app` этого инстанса

### Requirement: Сканер workspace
Система SHALL сканировать только непосредственные подпапки (1 уровень), пропуская скрытые и переходя по симлинкам на каталоги. Каждая подпапка классифицируется: GitLab-проект (remote на хост инстанса), чужой remote, без `.git`.

#### Scenario: Смешанная папка
- **WHEN** в папке есть `a` (remote на инстанс), `b` (remote на github.com), `notes` (без `.git`), `.cache`
- **THEN** `a` — GitLab-проект, `b` — чужой remote, `notes` — без `.git`, `.cache` не рассматривается
