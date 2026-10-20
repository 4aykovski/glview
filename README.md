# gitlab-tui

Терминальный клиент GitLab на Go и [Bubble Tea](https://github.com/charmbracelet/bubbletea). Показывает MR, пайплайны, логи джоб, теги и issues текущего проекта или целой папки с проектами и делает частые действия без браузера: approve, merge, rebase, retry/cancel/play джоб, запуск пайплайна с переменными.

Self-hosted GitLab — первоклассный сценарий: свой URL, relative URL root, нестандартный SSH-порт, свой CA.

## Возможности

- **Режим проекта.** Запуск в папке с `.git` сразу открывает проект по `remote origin`: MR, пайплайны, теги, issues.
- **Режим workspace.** Запуск в папке с проектами — общие списки MR, пайплайнов и тегов по всем проектам; нагрузка на GitLab — один GraphQL-запрос на пачку проектов.
- **Действия над MR:** approve/unapprove, merge (сейчас или auto-merge, squash, удаление ветки), rebase, checkout через `glab`.
- **Пайплайны:** стейджи, джобы, downstream-пайплайны, retry/cancel, play manual-джобы с переменными, запуск нового пайплайна с подсказками переменных из `.gitlab-ci.yml`.
- **Лог джобы в реальном времени:** ANSI-цвета, сворачиваемые секции, поиск, follow.
- **Русский и английский интерфейс** с переключением на лету (`ctrl+l`).
- **Цвета терминала:** только 16 ANSI-цветов, светлая и тёмная тема, `NO_COLOR`.

## Статус

Проект в разработке. Готово: каркас (M0) и режим проекта (M1). Дальше — workspace и поиск (M2), теги, issues и полировка (M3).

План и прогресс — в [Дорожной карте](https://github.com/4aykovski/glview/issues/36) и [вехах](https://github.com/4aykovski/glview/milestones).

## Установка

Нужен Go 1.26+.

```sh
go install github.com/4aykovski/glview/cmd/gitlab-tui@latest
```

Или из исходников:

```sh
git clone https://github.com/4aykovski/glview.git
cd glview
make build        # бинарь в ./bin/gitlab-tui
```

Опционально:

- [`glab`](https://gitlab.com/gitlab-org/cli) — токен без отдельной настройки и checkout МРа (`C`);
- `git` — fetch и pull локальных клонов (с M2).

## Запуск

```sh
cd ~/src/route-service && gitlab-tui      # режим проекта: проект из remote origin
cd ~/src && gitlab-tui                    # режим workspace: подпапки с .git на один уровень
gitlab-tui -p platform/route-service      # проект напрямую
gitlab-tui -p https://gitlab.example.com/platform/route-service
```

| Флаг | Значение |
|---|---|
| `-p, --project <path\|url>` | открыть проект напрямую: `group/sub/project` или полный URL проекта |
| `--config <file>` | другой конфиг |
| `--debug` | лог в `$XDG_CACHE_HOME/gitlab-tui/debug.log`, токены маскируются |
| `--debug-styles` | показать все стили и статусы темы и выйти |
| `--version` | версия и выход |

### Первый запуск и токен

Хост и токен ищутся по цепочке:

- **хост:** URL в `-p` → `host` в конфиге → `GITLAB_HOST` → хост из `remote origin`;
- **токен:** `token` в конфиге → `GITLAB_TOKEN` → `glab` для этого хоста.

Если чего-то не хватает, откроется экран настройки: URL инстанса и Personal Access Token (или «взять токен из glab»). Нужен scope `api`; с `read_api` работает только просмотр. После проверки (`GET /user`) настройки записываются в конфиг с правами `0600`. Токен из `glab` в конфиг не сохраняется; токен нигде не печатается и не попадает в debug-лог.

## Конфиг

`$XDG_CONFIG_HOME/gitlab-tui/config.yaml`, по умолчанию `~/.config/gitlab-tui/config.yaml` (и на macOS). Все ключи необязательные:

```yaml
host: https://gitlab.example.com     # URL инстанса; можно с path-prefix: https://example.com/gitlab
token: ""                            # PAT; пусто → env GITLAB_TOKEN → glab
host_aliases: [gitlab-work]          # SSH-алиасы из ~/.ssh/config, которые означают этот хост
language: en                         # en | ru

tls:
  ca_file: ""                        # свой CA для self-hosted
  insecure_skip_verify: false

project:
  page_size: 20

run_pipeline:                        # форма Run pipeline
  refs_first: branches               # branches | tags — какая группа первой в подсказках ref
  prefill_local_branch: true         # подставить текущую локальную ветку и поставить её первой

polling:
  active: 3s                         # пока есть running-пайплайны
  idle: 30s

glab:
  enabled: auto                      # auto | false
  path: ""                           # пусто → поиск в PATH
```

Полный список ключей — [SPEC §7](docs/SPEC.md#7-конфиг).

## Хоткеи

Хоткеи регистрозависимы; `?` показывает хоткеи текущего экрана.

| Где | Клавиши |
|---|---|
| Везде | `j/k` `↓/↑` ход, `g/G` начало/конец, `enter` открыть, `esc` назад, `/` фильтр, `r` обновить, `o` в браузере, `y` копировать ссылку, `m` фильтр «я», `1–4` и `tab` табы, `ctrl+l` язык, `?` хелп, `q` выход |
| MRs и Pipelines | `s` главный фильтр по кругу, `backspace` сбросить фильтры |
| MR (список и детали) | `a` approve/unapprove, `M` merge, `b` rebase, `C` checkout через glab |
| MR, вкладка Discussions | `J/K` между тредами, `space` свернуть тред |
| Pipelines | `enter` пайплайн, `l` лог упавшей джобы, `n` новый пайплайн, `R` retry, `c` cancel, `O` пайплайн в браузере |
| Pipeline | `h/l` стейдж, `j/k` джоба, `enter` лог или downstream, `P` play, `R` retry, `c` cancel |
| Job log | `f` follow, `space` свернуть секцию, `z` свернуть все, `/` поиск, `n/N` совпадения |

Полная таблица, включая будущие workspace, теги и issues, — [SPEC §3.1](docs/SPEC.md#31-сводная-таблица-хоткеев).

## Разработка

```sh
make build         # бинарь в ./bin/gitlab-tui
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
