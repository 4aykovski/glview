# glview

**English** | [Русский](README.ru.md)

A terminal GitLab client written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea). It shows MRs with diffs, pipelines, job logs, tags, issues and To-Dos for the current project or a whole folder of projects, and runs the common actions without a browser: create MRs, approve, merge, rebase, comment (including on diff lines), retry/cancel/play jobs, run pipelines with variables, create and delete tags, close/reopen issues.

Self-hosted GitLab is fully supported: custom URL, relative URL root, non-standard SSH port, custom CA.

## Features

- **Project mode.** Run it in a folder with `.git` and the project from `remote origin` opens right away: MRs, pipelines, tags, issues.
- **Workspace mode.** Run it in a folder of projects to get Projects, MRs, Pipelines and Tags tabs across all projects, grouped by project. GitLab load is one GraphQL request per batch of projects. `f` opens the project of the selected row, `P` filters by project, `u`/`U` fetch and pull (ff-only) the local clones.
- **Search.** `p` switches projects without network access (git remotes, workspace, starred, recent). `F` searches projects and groups on the server; a group opens as a workspace of its projects.
- **MRs:** approve/unapprove, merge (now or auto-merge, squash, delete branch), rebase, comments (`c`), checkout via `glab`, threads as cards, tracker links in the title and description (`L`).
- **Diffs.** The Diff tab of an MR shows all files in one scroll with line numbers, collapsible files and threads under their lines. `c` on a line comments on that line; `d` in the MR list opens the MR straight on the Diff tab.
- **New MR** (`n` in the MRs tab): branches with suggestions, title with a Draft toggle, description from project templates, assignees, reviewers, labels, milestone, remove source branch and squash. Before creating, it checks whether an open MR for the same branch pair exists, whether there are commits, and whether the branch is pushed.
- **Pipelines:** stages, jobs, downstream pipelines, retry/cancel, play manual jobs with variables, run a new pipeline with variable hints from `.gitlab-ci.yml`.
- **Live job log:** ANSI colors, collapsible sections, search, follow.
- **Tags.** A list with each tag's pipeline and comparison to the default branch, a New tag form (`n`: next semver version, ref, message, commits in the tag), deletion (`D`).
- **Issues.** A list with filters and preview, related MRs, an issue screen with threads, comments, close/reopen (`x`).
- **To-Do** (`T` from any screen): your to-do items with state and type filters, jump to the target, mark one (`x`) or all (`X`) as done; a To-Do counter in the header.
- **English and Russian UI**, switchable on the fly (`ctrl+l`).
- **Terminal colors:** only the 16 ANSI colors, light and dark themes, `NO_COLOR`.
- **Network.** A token rejected mid-session stops polling and tells you where to fix it. Offline, screens show cached data marked `(offline)` and polling slows down until the connection is back.

## Status

The current version is [v0.1.0](https://github.com/4aykovski/glview/releases). It includes all MVP milestones (foundation M0, project mode M1, workspace, GraphQL batching and search M2, tags, issues, forms and polish M3) and the first post-MVP tasks: diffs and line comments, MR creation, To-Do.

Known limitations and plans are in the [roadmap](https://github.com/4aykovski/glview/issues/36), [milestones](https://github.com/4aykovski/glview/milestones) and open [issues](https://github.com/4aykovski/glview/issues).

## Installation

Requires Go 1.26+.

```sh
go install github.com/4aykovski/glview/cmd/glview@latest
```

Or from source:

```sh
git clone https://github.com/4aykovski/glview.git
cd glview
make build        # binary in ./bin/glview
```

Optional:

- [`glab`](https://gitlab.com/gitlab-org/cli) provides a token without extra setup and MR checkout (`C`);
- `git` is used for fetch and pull (ff-only) of local clones in a workspace (`u`/`U`). It runs without a terminal, so remote access must be non-interactive: ssh-agent, keychain or a credential helper.

## Usage

```sh
cd ~/src/route-service && glview      # project mode: project from remote origin
cd ~/src && glview                    # workspace mode: subfolders with .git, one level deep
glview -p platform/route-service      # open a project directly
glview -p https://gitlab.example.com/platform/route-service
```

In a workspace, `F` searches projects and groups on the server; the selected group opens as a workspace of its projects (up to `workspace.max_group_projects`), `esc` goes back. In a project, `p` switches projects without leaving the TUI.

The minimum terminal size is 80×24; anything smaller shows "Make the terminal at least 80×24". `?` shows the hotkeys of the current screen; if the list is taller than the terminal, it scrolls (`j/k`).

| Flag | Meaning |
|---|---|
| `-p, --project <path\|url>` | open a project directly: `group/sub/project` or the full project URL |
| `--config <file>` | use another config file |
| `--debug` | log to `$XDG_CACHE_HOME/glview/debug.log`, tokens are masked |
| `--debug-styles` | print all theme styles and statuses and exit |
| `--version` | print version and exit |

### First run and token

Host and token are looked up in order:

- **host:** URL in `-p` → `host` in the config → `GITLAB_HOST` → host from `remote origin`;
- **token:** `token` in the config → `GITLAB_TOKEN` → `glab` for this host.

If something is missing, a setup screen opens: instance URL and Personal Access Token (or "take the token from glab"). The `api` scope is needed; with `read_api` only viewing works. After a check (`GET /user`) the settings are written to the config with `0600` permissions. A token from `glab` is not saved to the config; the token is never printed and never reaches the debug log.

## Config

`$XDG_CONFIG_HOME/glview/config.yaml`, by default `~/.config/glview/config.yaml` (macOS too). All keys are optional:

```yaml
host: https://gitlab.example.com     # instance URL; a path prefix is fine: https://example.com/gitlab
token: ""                            # PAT; empty → env GITLAB_TOKEN → glab
host_aliases: [gitlab-work]          # SSH aliases from ~/.ssh/config that mean this host
language: en                         # en | ru
theme: terminal                      # terminal | terminal-faint (see "Themes")

tls:
  ca_file: ""                        # custom CA for self-hosted
  insecure_skip_verify: false

workspace:
  per_project: 3                     # entries per project in the shared tabs
  max_group_projects: 100            # project limit for a workspace from a group

project:
  page_size: 20

run_pipeline:                        # Run pipeline form
  refs_first: branches               # branches | tags — which group comes first in ref hints
  prefill_local_branch: true         # prefill the current local branch and put it first

graphql:
  batch_size: 10                     # initial batch size, adaptive afterwards

polling:
  active: 3s                         # while there are running pipelines
  idle: 30s
  workspace: 60s

glab:
  enabled: auto                      # auto | false
  path: ""                           # empty → look up in PATH

git:
  enabled: auto                      # auto | false
  path: ""                           # empty → look up in PATH
  parallel: 6                        # concurrent fetch/pull
  timeout: 60s                       # per repository
  fetch_on_start: false              # background fetch of all workspace projects on start

links:                               # custom links in MR title and description (L key)
  - pattern: '[A-Z][A-Z0-9]+-\d+'     # Go regular expression (RE2)
    url: 'https://tracker.example.com/browse/$0'  # $0 — whole match, $1…, ${name} — groups
```

An unknown value (`theme: foo`) is replaced with the default and a warning at startup; invalid YAML does not abort the start. `links` rules are for trackers without a GitLab integration (with an integration, GitLab itself provides the links); a broken rule is skipped with a warning. The full list of keys is in [SPEC §7](docs/SPEC.md#7-конфиг) (in Russian).

## Themes

The UI uses only the 16 ANSI colors, that is, the terminal's own palette; a light or dark background is detected at startup.

- `theme: terminal` (default) — secondary text and borders in ANSI 8 ("bright black"). In Solarized Dark this color matches the background and the text disappears.
- `theme: terminal-faint` — secondary text in the terminal's default color with the Faint attribute, borders in ANSI 0 on a dark background. Fits palettes where ANSI 8 blends into the background; not all terminals render Faint the same way.
- `NO_COLOR=1` — colors are off, bold and reverse remain: the selection and the active tab stay visible, statuses are told apart by glyphs.

`glview --debug-styles` prints a sample of all styles of the selected theme, handy for comparing themes on your palette.

## Hotkeys

Hotkeys are case-sensitive; `?` shows the hotkeys of the current screen.

| Where | Keys |
|---|---|
| Everywhere | `j/k` `↓/↑` move, `g/G` top/bottom, `enter` open, `esc` back, `/` filter/search, `r` refresh, `o` open in browser, `y` copy link, `m` "mine" filter, `1–4` and `tab` tabs, `p` project picker, `F` global search, `T` To-Do, `ctrl+l` language, `?` help, `q` quit |
| Workspace | `J/K` between groups, `space` and `enter` collapse/expand a group, `P` pick projects, `f` open the row's project, `u` fetch all, `U` pull ff-only all |
| `p` and `F` overlays | `↓/↑` `ctrl+n/ctrl+p` move, `ctrl+d/ctrl+u` and `pgdown/pgup` half a page, `enter` open, `esc` close |
| MRs and Pipelines | `s` cycle the main filter, `backspace` reset filters |
| MR (list and detail) | `a` approve/unapprove, `M` merge, `b` rebase, `C` checkout via glab, `L` MR links, `A` all approvers, `c` comment |
| MRs tab | `n` new MR, `d` MR on the Diff tab, `O` MR in browser, `o` project MR list with filters |
| MR, Diff tab | `J/K` between files, `←/→` horizontal scroll, `space` collapse file, `z` collapse all, `c` comment on line, `enter` on a thread — thread on Discussions, `o` file in browser |
| MR, Discussions tab | `J/K` between threads, `space` collapse thread |
| Pipelines | `enter` pipeline, `l` log of the failed job, `n` new pipeline, `R` retry, `c` cancel, `O` pipeline in browser |
| Pipeline | `h/l` stage, `j/k` job, `enter` log or downstream, `a` attempts, `P` play, `R` retry, `c` cancel |
| Job log | `f` follow, `space` collapse section, `z` collapse all, `/` search, `n/N` matches, `[` `]` attempts |
| Tags | `n` new tag, `D` delete, `enter` tag pipeline, `y` copy name |
| Issues | `enter` issue screen, `c` comment, `x` close/reopen, `s` cycle state, `O` issue in browser |
| Issue | `J/K` thread, `space` collapse thread, `c` comment, `x` close/reopen |
| To-Do | `enter` open target, `x` mark done, `X` mark all done, `s` cycle state, `t` cycle type, `O` target in browser |
| Project with a local folder | `u` fetch, `U` pull ff-only |
| New MR form | `tab`/`shift+tab` field, `↑/↓` suggestion, `enter` take suggestion or next field, `space` checkbox, `enter` on the button — create, `esc` close (draft is kept) |
| Comment form | `tab` to the button, `alt+enter` or `enter` on the button — send, `esc` close (draft is kept until the end of the session) |

In input fields single-letter hotkeys are off; `esc` leaves the field.

The full table is in [SPEC §3.1](docs/SPEC.md#31-сводная-таблица-хоткеев) (in Russian).

## Development

```sh
make build         # binary in ./bin/glview
make test          # go test ./...
make lint          # golangci-lint run
make run ARGS="-p platform/route-service"
make hooks         # enable git hooks that check for confidential data (once after cloning)
make confidential  # check all repository files with the same rules
```

Project documentation is in Russian:

- [docs/SPEC.md](docs/SPEC.md) — specification: behavior, hotkeys, API, architecture.
- [GitHub Issues](https://github.com/4aykovski/glview/issues) — tasks and milestones, roadmap — [#36](https://github.com/4aykovski/glview/issues/36).
- [docs/design/](docs/design/README.md) — screen mockups.
- [CLAUDE.md](CLAUDE.md) — project rules, including for AI agents.
- Change planning — [OpenSpec](openspec/): one change per milestone.

The repository is public: code, tests, fixtures and issues contain only made-up data (`gitlab.example.com`, `platform/route-service`). Before committing — `make test lint confidential`; the hooks in `scripts/githooks` must not be bypassed.

Stack: Go 1.26, Bubble Tea v2, Bubbles v2, Lip Gloss v2, Huh v2, Glamour v2, [client-go](https://gitlab.com/gitlab-org/api/client-go) for REST and a custom GraphQL client.
