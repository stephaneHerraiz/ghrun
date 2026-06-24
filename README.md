# ghrun

**ghrun** is an interactive terminal UI (TUI) to **launch** and **monitor** GitHub
Actions workflows across multiple repositories, without leaving your terminal. It
builds on your already-authenticated `gh` CLI — no token to configure.

Written in Go with [Bubbletea](https://github.com/charmbracelet/bubbletea).

```
ghrun › owner/repo › Workflows › launch: deploy.yml
```

---

## Table of contents

- [Overview](#overview)
- [Requirements](#requirements)
- [Installation](#installation)
- [First run](#first-run)
- [Screens](#screens)
- [Keybindings](#keybindings)
- [Features](#features)
- [Configuration](#configuration)
- [How ghrun talks to GitHub](#how-ghrun-talks-to-github)
- [Out of scope](#out-of-scope)
- [License](#license)

---

## Overview

Two equally-weighted use cases:

- **Launch** a `workflow_dispatch` workflow: ghrun reads the workflow's `inputs`
  straight from its YAML and renders a **dynamic form** (text, number, boolean,
  choice), then dispatches on the branch you pick. After launching, it
  **automatically** switches to the detail view of the run it created.
- **Monitor** runs **live**: a multi-repo dashboard aggregates the status of your
  favorite repos, with job/step drill-down, scrollable logs and actions (rerun,
  rerun-failed, cancel, open in the browser).

Navigation uses a **screen stack** (push/pop). Convention: **uppercase = global
navigation** (available everywhere), **lowercase = contextual actions**.

---

## Requirements

- **An authenticated [`gh`](https://cli.github.com/)**: run `gh auth login` first.
  ghrun checks `gh auth status` on startup and exits with a clear message if you
  are not authenticated. Required scopes: `repo` and `workflow` (plus `read:org`
  to list organizations).
- **Go 1.25+** only if you install or build from source.

---

## Installation

Via `go install`:

```bash
go install github.com/stephaneHerraiz/ghrun/cmd/ghrun@latest
```

Or from source:

```bash
git clone https://github.com/stephaneHerraiz/ghrun.git
cd ghrun
go build -o ghrun ./cmd/ghrun
./ghrun
```

---

## First run

1. ghrun verifies that `gh` is authenticated.
2. On the very first run, an empty config file is created at
   `~/.config/ghrun/config.yaml`.
3. Since no default organization is set yet, ghrun shows an **organization
   picker** listing your personal account followed by the organizations you
   belong to. Your choice is **persisted** to the config.
4. You then land on the **dashboard**.

---

## Screens

| Screen | Purpose |
|---|---|
| **Organization picker** | First run only: choose the default account/organization. |
| **Dashboard** (home) | Hybrid list: favorites (with live status) + the organization's repos. Doubles as a filterable repo selector. |
| **Workflows** | A repo's workflows. `Enter` loads the `inputs` and opens the launch screen. |
| **Launch** | Pick the branch (`ref`), then a dynamic `inputs` form → dispatch. |
| **Runs** | Live list of the current repo's runs. |
| **Run detail** | Jobs and steps with status icons, refreshed live. |
| **Logs** | Scrollable viewport of the logs (`--log` or `--log-failed`). |

Every screen shows a **breadcrumb** in the header and a **footer** with the
keybindings and an error area (errors from `gh` show in red without blocking the
UI).

---

## Keybindings

### Global navigation (from any screen)

| Key | Action |
|---|---|
| `W` | Workflows of the current repo |
| `U` | Runs of the current repo |
| `R` | Back to home / repo selection |
| `Esc` | Pop one level off the stack |
| `?` | Show / hide help |
| `q` · `Ctrl-C` | Quit |

> `W` and `U` only take effect once a repo is selected.

### Dashboard (home)

| Key | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · mouse wheel | Move through the list |
| `Enter` | Enter the repo (opens its runs) |
| `f` | **Toggle favorite** for the highlighted repo (persisted to config) |
| `g` | Refresh (favorites + organization repos) |
| `/` | Filter the list (then type; `Enter` enters the repo, `Esc` clears the filter) |

### Workflows

| Key | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · mouse wheel | Move |
| `Enter` | Load inputs and configure the launch |

### Launch — branch selection

| Key | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · mouse wheel | Choose the branch (`main`/`master` pre-selected) |
| `Enter` | Continue to the inputs form |

### Launch — inputs form

| Key | Action |
|---|---|
| `↑`/`↓` | Move between fields |
| `←`/`→` | Change a choice or a boolean |
| *(typing)* | Type directly into a text/number field |
| `Enter` | **Launch** the workflow (`Ctrl-S` also works, as a fallback) |

### Runs

| Key | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · mouse wheel | Move |
| `Enter` | Open the run detail |
| `r` | Rerun the run |
| `f` | Rerun only the failed jobs |
| `x` | Cancel the run |
| `o` | Open the run in the browser |
| `g` | Refresh |

### Run detail

| Key | Action |
|---|---|
| `l` | View logs |
| `r` | Rerun · `f` rerun-failed · `x` cancel · `o` open web |

### Logs

| Key | Action |
|---|---|
| `↑`/`↓` · `PgUp`/`PgDn` · mouse wheel | Scroll the viewport |
| `Esc` | Back |

---

## Features

- **Hybrid dashboard**: your favorite repos (with last run, state and number of
  active runs, live) followed by the chosen organization's repos.
- **On-the-fly favorites**: `f` pins/unpins the highlighted repo; the change is
  written to the config immediately. An unpinned repo that belongs to the
  organization automatically reappears in its list.
- **Adaptive live refresh**: fast polling (~3-5s) while a run is active, slowed
  down (~15s) otherwise. A single global ticker avoids hammering the `gh` API.
- **Instant filtering** of repo lists (`/`).
- **Pagination + vertical scrollbar + mouse wheel** on every list (repos,
  workflows, runs, branches). The height is configurable (`listPageSize`).
- **Dynamic inputs form**: the `workflow_dispatch` `inputs` are parsed from the
  workflow YAML. Supported types: `string`, `number`, `boolean`, `choice`. Empty
  `required` fields block the launch with a message.
- **Branch pre-selection**: `main` then `master` are surfaced first.
- **Auto-switch after dispatch**: `gh workflow run` does not return the created
  run's ID; ghrun polls the run list right after (with a few retries) and opens
  the triggered run's detail directly.
- **Job/step drill-down** with status icons (success / failure / in progress /
  queued).
- **Scrollable logs** full-screen (`--log`, or `--log-failed` when the run
  failed).
- **Run actions**: rerun, rerun-failed, cancel, open in the browser.
- **Mouse** enabled everywhere (wheel scrolling on lists and logs).
- **Non-blocking errors**: `gh` failures show in red in the footer and clear after
  a few seconds; only an auth failure at startup is fatal.

---

## Configuration

File: `~/.config/ghrun/config.yaml` (honors `XDG_CONFIG_HOME`).

```yaml
defaultOrg: stephaneHerraiz       # account/org used to list repos
refreshIntervalSeconds: 4         # base cadence of the live refresh
runListLimit: 30                  # number of runs fetched per repo
listPageSize: 20                  # rows shown per list (+ scrollbar)
favorites:                        # pinned repos (live status on the home screen)
  - stephaneHerraiz/ghrun
  - owner/another-repo
```

| Key | Default | Description |
|---|---|---|
| `defaultOrg` | *(chosen on first run)* | Account or organization whose repos are listed. |
| `refreshIntervalSeconds` | `4` | Base cadence of the live refresh. |
| `runListLimit` | `30` | Number of runs fetched per repo. |
| `listPageSize` | `20` | List height (in rows) before pagination/scrollbar. |
| `favorites` | `[]` | List of pinned `owner/name` repos. Also editable via the `f` key. |

**Cache**: the organization's repo list is cached at `~/.cache/ghrun/repos.json`
(honors `XDG_CACHE_HOME`) for instant display, then refreshed in the background.

---

## How ghrun talks to GitHub

ghrun invents nothing: it wraps the `gh` CLI (and its API). Overview:

| Function | Underlying command |
|---|---|
| Auth (startup) | `gh auth status` |
| Organizations / account | `gh api user`, `gh api user/orgs` |
| Repo list | `gh repo list <org> --json nameWithOwner` |
| A repo's workflows | `gh workflow list` |
| A workflow's inputs | the YAML file contents → `on.workflow_dispatch.inputs` |
| Branches | `gh api repos/{o}/{r}/branches` |
| **Dispatch** | `gh workflow run {id} --ref {branch} -f key=value …` |
| Runs (live list) | `gh run list` |
| Run detail | `gh run view {id} --json status,conclusion,jobs` |
| Logs | `gh run view {id} --log` / `--log-failed` |
| rerun / rerun-failed / cancel | `gh run rerun {id}` / `--failed` / `gh run cancel {id}` |
| Open web | `gh run view {id} --web` |

All calls are **asynchronous** (via Bubbletea commands): the UI never blocks.

---

## Out of scope

Deliberately not covered (YAGNI):

- Editing workflows or secrets.
- Artifact management / download.
- Multi-host GitHub Enterprise (ghrun targets `github.com` via the existing `gh`
  auth).
- System notifications.

---

## License

[MIT](LICENSE) © 2026 Stéphane Herraiz
