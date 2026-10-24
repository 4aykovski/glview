# glview

Терминальный клиент GitLab на Go и [Bubble Tea](https://github.com/charmbracelet/bubbletea). Показывает MR, пайплайны, логи джоб, теги и issues текущего проекта или целой папки с проектами и делает частые действия без браузера: approve, merge, rebase, комментарии, retry/cancel/play джоб, запуск пайплайна с переменными, создание и удаление тегов, close/reopen issues.

Self-hosted GitLab — первоклассный сценарий: свой URL, relative URL root, нестандартный SSH-порт, свой CA.

## Возможности

- **Режим проекта.** Запуск в папке с `.git` сразу открывает проект по `remote origin`: MR, пайплайны, теги, issues.
- **Режим workspace.** Запуск в папке с проектами — табы «Проекты», MRs, Pipelines и Tags по всем проектам сразу, сгруппированные по проектам; нагрузка на GitLab — один GraphQL-запрос на пачку проектов. `f` открывает проект выбранной строки, `P` фильтрует по проектам, `u`/`U` делают fetch и pull (ff-only) локальных клонов.
- **Поиск.** `p` — переключатель проектов без сети (git remote, workspace, starred, недавние), `F` — глобальный поиск проектов и групп; группа открывается как workspace из её проектов.
- **MR:** approve/unapprove, merge (сейчас или auto-merge, squash, удаление ветки), rebase, комментарии (`c`), checkout через `glab`, треды в виде карточек, ссылки на трекер в заголовке и описании (`L`).
- **Пайплайны:** стейджи, джобы, downstream-пайплайны, retry/cancel, play manual-джобы с переменными, запуск нового пайплайна с подсказками переменных из `.gitlab-ci.yml`.
- **Лог джобы в реальном времени:** ANSI-цвета, сворачиваемые секции, поиск, follow.
- **Теги.** Список с пайплайном тега и сравнением с default-веткой, форма New tag (`n`: следующая semver-версия, ref, сообщение, коммиты в теге), удаление (`D`).
- **Issues.** Список с фильтрами и превью, связанные MR, экран issue с тредами, комментарии, close/reopen (`x`).
- **Русский и английский интерфейс** с переключением на лету (`ctrl+l`).
- **Цвета терминала:** только 16 ANSI-цветов, светлая и тёмная тема, `NO_COLOR`.
- **Сеть.** Токен, отвергнутый посреди сессии, останавливает поллинг и подсказывает, где его поправить; без сети экраны показывают данные из кэша и пометку `(офлайн)`, поллинг замедляется до восстановления.

## Статус

Реализованы все вехи MVP: каркас (M0), режим проекта (M1), workspace, GraphQL-батчинг и поиск (M2), теги, issues, формы и полировка (M3). Остаётся ручная проверка на живых инстансах и разных терминалах — чек-лист в [#25](https://github.com/4aykovski/glview/issues/25).

План и прогресс — в [Дорожной карте](https://github.com/4aykovski/glview/issues/36) и [вехах](https://github.com/4aykovski/glview/milestones).

## Установка

Нужен Go 1.26+.

```sh
go install github.com/4aykovski/glview/cmd/glview@latest
```

Или из исходников:

```sh
git clone https://github.com/4aykovski/glview.git
cd glview
make build        # бинарь в ./bin/glview
```

Опционально:

- [`glab`](https://gitlab.com/gitlab-org/cli) — токен без отдельной настройки и checkout МРа (`C`);
- `git` — fetch и pull (ff-only) локальных клонов в workspace (`u`/`U`); запускается без терминала, поэтому нужен неинтерактивный доступ к remote: ssh-agent, keychain или credential helper.

## Запуск

```sh
cd ~/src/route-service && glview      # режим проекта: проект из remote origin
cd ~/src && glview                    # режим workspace: подпапки с .git на один уровень
glview -p platform/route-service      # проект напрямую
glview -p https://gitlab.example.com/platform/route-service
```

В workspace `F` ищет проекты и группы на сервере; выбранная группа открывается как workspace из её проектов (до `workspace.max_group_projects`), `esc` возвращает назад. В проекте `p` переключает проект, не выходя из TUI.

Минимальный размер терминала — 80×24; меньше — сообщение «расширь терминал». Справка по клавишам текущего экрана — `?`: если список выше терминала, он прокручивается (`j/k`).

| Флаг | Значение |
|---|---|
| `-p, --project <path\|url>` | открыть проект напрямую: `group/sub/project` или полный URL проекта |
| `--config <file>` | другой конфиг |
| `--debug` | лог в `$XDG_CACHE_HOME/glview/debug.log`, токены маскируются |
| `--debug-styles` | показать все стили и статусы темы и выйти |
| `--version` | версия и выход |

### Первый запуск и токен

Хост и токен ищутся по цепочке:

- **хост:** URL в `-p` → `host` в конфиге → `GITLAB_HOST` → хост из `remote origin`;
- **токен:** `token` в конфиге → `GITLAB_TOKEN` → `glab` для этого хоста.

Если чего-то не хватает, откроется экран настройки: URL инстанса и Personal Access Token (или «взять токен из glab»). Нужен scope `api`; с `read_api` работает только просмотр. После проверки (`GET /user`) настройки записываются в конфиг с правами `0600`. Токен из `glab` в конфиг не сохраняется; токен нигде не печатается и не попадает в debug-лог.

## Конфиг

`$XDG_CONFIG_HOME/glview/config.yaml`, по умолчанию `~/.config/glview/config.yaml` (и на macOS). Все ключи необязательные:

```yaml
host: https://gitlab.example.com     # URL инстанса; можно с path-prefix: https://example.com/gitlab
token: ""                            # PAT; пусто → env GITLAB_TOKEN → glab
host_aliases: [gitlab-work]          # SSH-алиасы из ~/.ssh/config, которые означают этот хост
language: en                         # en | ru
theme: terminal                      # terminal | terminal-faint (см. «Темы»)

tls:
  ca_file: ""                        # свой CA для self-hosted
  insecure_skip_verify: false

workspace:
  per_project: 3                     # записей на проект в общих табах
  max_group_projects: 100            # лимит проектов для workspace из группы

project:
  page_size: 20

run_pipeline:                        # форма Run pipeline
  refs_first: branches               # branches | tags — какая группа первой в подсказках ref
  prefill_local_branch: true         # подставить текущую локальную ветку и поставить её первой

graphql:
  batch_size: 10                     # стартовый размер пачки, дальше адаптивно

polling:
  active: 3s                         # пока есть running-пайплайны
  idle: 30s
  workspace: 60s

glab:
  enabled: auto                      # auto | false
  path: ""                           # пусто → поиск в PATH

git:
  enabled: auto                      # auto | false
  path: ""                           # пусто → поиск в PATH
  parallel: 6                        # одновременных fetch/pull
  timeout: 60s                       # на один репозиторий
  fetch_on_start: false              # фоновый fetch всех проектов workspace при старте

links:                               # свои ссылки в заголовке и описании MR (клавиша L)
  - pattern: '[A-Z][A-Z0-9]+-\d+'     # регулярное выражение Go (RE2)
    url: 'https://tracker.example.com/browse/$0'  # $0 — всё совпадение, $1…, ${name} — группы
```

Неизвестное значение (`theme: foo`) заменяется значением по умолчанию с предупреждением при старте; невалидный YAML не прерывает запуск. Правила `links` нужны трекерам без интеграции с GitLab (с интеграцией ссылки приходят от самого GitLab); битое правило пропускается с предупреждением. Полный список ключей — [SPEC §7](docs/SPEC.md#7-конфиг).

## Темы

Интерфейс использует только 16 ANSI-цветов, то есть палитру самого терминала; светлый или тёмный фон определяется при старте.

- `theme: terminal` (по умолчанию) — второстепенный текст и рамки в ANSI 8 («bright black»). В Solarized Dark этот цвет совпадает с фоном и текст пропадает.
- `theme: terminal-faint` — второстепенный текст цветом терминала по умолчанию с атрибутом Faint, рамки на тёмном фоне в ANSI 0. Подходит для палитр, где ANSI 8 сливается с фоном; не все терминалы одинаково рисуют Faint.
- `NO_COLOR=1` — цвета отключаются, остаются жирный и инверсия: выделение и активный таб видны, статусы различимы по глифам.

`glview --debug-styles` печатает образец всех стилей выбранной темы — удобно сравнить темы на своей палитре.

## Хоткеи

Хоткеи регистрозависимы; `?` показывает хоткеи текущего экрана.

| Где | Клавиши |
|---|---|
| Везде | `j/k` `↓/↑` ход, `g/G` начало/конец, `enter` открыть, `esc` назад, `/` фильтр/поиск, `r` обновить, `o` в браузере, `y` копировать ссылку, `m` фильтр «я», `1–4` и `tab` табы, `p` выбор проекта, `F` глобальный поиск, `T` To-Do, `ctrl+l` язык, `?` хелп, `q` выход |
| Workspace | `J/K` между группами, `space` и `enter` свернуть/развернуть группу, `P` выбор проектов, `f` открыть проект строки, `u` fetch всех, `U` pull ff-only всех |
| Оверлеи `p` и `F` | `↓/↑` `ctrl+n/ctrl+p` ход, `ctrl+d/ctrl+u` и `pgdown/pgup` на полстраницы, `enter` открыть, `esc` закрыть |
| MRs и Pipelines | `s` главный фильтр по кругу, `backspace` сбросить фильтры |
| MR (список и детали) | `a` approve/unapprove, `M` merge, `b` rebase, `C` checkout через glab, `L` ссылки MR, `c` комментарий |
| Таб MRs | `O` MR в браузере, `o` список MR проекта с фильтрами |
| MR, вкладка Discussions | `J/K` между тредами, `space` свернуть тред |
| Pipelines | `enter` пайплайн, `l` лог упавшей джобы, `n` новый пайплайн, `R` retry, `c` cancel, `O` пайплайн в браузере |
| Pipeline | `h/l` стейдж, `j/k` джоба, `enter` лог или downstream, `a` попытки, `P` play, `R` retry, `c` cancel |
| Job log | `f` follow, `space` свернуть секцию, `z` свернуть все, `/` поиск, `n/N` совпадения, `[` `]` попытки |
| Tags | `n` новый тег, `D` удалить, `enter` пайплайн тега, `y` копировать имя |
| Issues | `enter` экран issue, `c` комментарий, `x` close/reopen, `s` state по кругу, `O` issue в браузере |
| Issue | `J/K` тред, `space` свернуть тред, `c` комментарий, `x` close/reopen |
| To-Do | `enter` открыть цель, `x` отметить выполненной, `X` отметить все, `s` state по кругу, `t` тип по кругу, `O` цель в браузере |
| Проект с локальной папкой | `u` fetch, `U` pull ff-only |
| Форма комментария | `tab` к кнопке, `alt+enter` или `enter` на кнопке — отправить, `esc` закрыть (черновик хранится до конца сессии) |

В полях ввода однобуквенные хоткеи отключены, `esc` выходит из поля.

Полная таблица — [SPEC §3.1](docs/SPEC.md#31-сводная-таблица-хоткеев).

## Разработка

```sh
make build         # бинарь в ./bin/glview
make test          # go test ./...
make lint          # golangci-lint run
make run ARGS="-p platform/route-service"
make hooks         # включить git-хуки проверки конфиденциальности (один раз после клона)
make confidential  # проверить все файлы репозитория теми же правилами
```

- [docs/SPEC.md](docs/SPEC.md) — спецификация: поведение, хоткеи, API, архитектура.
- [GitHub Issues](https://github.com/4aykovski/glview/issues) — задачи и вехи, дорожная карта — [#36](https://github.com/4aykovski/glview/issues/36).
- [docs/design/](docs/design/README.md) — макеты экранов.
- [CLAUDE.md](CLAUDE.md) — правила проекта, в том числе для ИИ-агентов.
- Планирование изменений — [OpenSpec](openspec/): одна change на веху.

Репозиторий публичный: в коде, тестах, фикстурах и issues — только выдуманные данные (`gitlab.example.com`, `platform/route-service`). Перед коммитом — `make test lint confidential`; хуки из `scripts/githooks` обходить нельзя.

Стек: Go 1.26, Bubble Tea v2, Bubbles v2, Lip Gloss v2, Huh v2, Glamour v2, [client-go](https://gitlab.com/gitlab-org/api/client-go) для REST и свой GraphQL-клиент.
