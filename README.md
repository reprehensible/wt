# wt

[![Go](https://img.shields.io/github/go-mod/go-version/reprehensible/wt)](https://github.com/reprehensible/wt)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A CLI tool for managing git worktrees. Create, list, navigate, and delete
worktrees from the command line or an interactive TUI. Optionally integrate
with Jira to create worktrees from issues and keep statuses in sync.

Worktrees are created under `<repo>-worktrees/` alongside your main checkout.

## Use Cases

**Parallel feature development.** You're halfway through a refactor when a
critical bug report comes in. Instead of stashing your work or cloning the
repo again, you run `wt new hotfix-auth-crash`, step into a clean worktree,
fix the bug, and push — all without disturbing your refactor in progress.

**Code review with full context.** A teammate asks you to review their PR.
You `wt new their-branch`, open the worktree in your editor, run the tests
locally, and leave comments — while your own feature branch keeps running a
long test suite in the other worktree.

**Jira-driven workflow.** Your team assigns you PROJ-472. You run
`wt jira new PROJ-472` and the tool fetches the issue summary, generates a
branch name, creates the worktree, drops a markdown file with the issue
description and comments into it, and transitions the Jira ticket to
"In Progress" — all in one command.

**Keeping your environment intact.** Your project has `.env` files and AI
config (`CLAUDE.md`, `AGENTS.md`) that you don't want to recreate for every
worktree. By default, `wt` copies these into new worktrees automatically.
Need `node_modules` too? Add `-l` and skip the install step entirely.

**Tmux multiplexing.** You're juggling three features at once. Each one lives
in its own worktree and its own tmux session. `wt t feature-login` either
creates a new session or switches to the existing one — no need to remember
directory paths.

## AI Statement

This repository was created entirely with Claude Code, for two reasons:

 1. Because I wanted a tool that works exactly like this one does
 2. Because I wanted to try out some techniques with Claude Code

The big thing I wanted to try was requiring 100% test coverage. I can't really speak yet to how well it worked in terms of increasing robustness, but Claude did work pretty hard to meet the goal.

Given that this is a local development tool that I might be the only user of forever, I'm not too worried about it.

## Install

```bash
go install github.com/reprehensible/wt@latest
```

Or build from source:

```bash
git clone https://github.com/reprehensible/wt.git
cd wt
go build -o wt .
```

Requires Go 1.24+.

## Usage

```
wt                        # open interactive TUI
wt new <branch>           # create a new worktree
wt list                   # list worktrees
wt go <name>              # open a shell in a worktree
wt t <name>               # open a worktree in a tmux session
wt jira new <key>         # create a worktree from a Jira issue
wt jira status [key]      # view or set Jira issue status
wt jira status sync       # sync Jira status from GitHub PR state
wt jira config            # show or initialize Jira status mappings
```

### `wt new` options

| Flag | Description |
|------|-------------|
| `-c`, `--copy-config` | Copy config files (default: on) |
| `-C`, `--no-copy-config` | Skip copying config files |
| `-l`, `--copy-libs` | Copy libraries (default: off) |
| `-L`, `--no-copy-libs` | Skip copying libraries |
| `-f`, `--from <branch>` | Base branch to create from |

Config files copied by default: `.env`, `AGENTS.md`, `CLAUDE.md`.
Libraries (copied with `-l`): `node_modules`.

### `wt jira new` options

| Flag | Description |
|------|-------------|
| `-t` | Open worktree in tmux after creation |
| `-b`, `--branch <name>` | Override the auto-generated branch name |
| `-c`, `--copy-config` | Copy config files (default: on) |
| `-C`, `--no-copy-config` | Skip copying config files |
| `-l`, `--copy-libs` | Copy libraries (default: off) |
| `-L`, `--no-copy-libs` | Skip copying libraries |
| `-f`, `--from <branch>` | Base branch to create from |
| `-S`, `--no-status-update` | Skip auto-transitioning the issue to "working" |

The branch name is auto-generated from the issue key and summary
(e.g., `PROJ-123: Add login feature` becomes `proj-123-add-login-feature`).
A markdown file with the issue description and comments is written into the
worktree root.

### `wt jira status sync`

Syncs Jira issue status based on the state of the associated GitHub PR
(requires the `gh` CLI):

| PR state | Jira status |
|----------|-------------|
| Merged | `testing` |
| Open | `review` |
| Draft | _(no change)_ |
| No PR found | `working` |

Pass `-n` / `--dry-run` to preview changes without applying them.

### Examples

```bash
# Create a worktree for an existing branch
wt new feature-login

# Create a new branch from develop
wt new -f develop feature-login

# Skip copying config files
wt new -C my-branch

# Copy node_modules too
wt new -l my-branch

# Jump into a worktree
wt go feature-login

# Open in tmux
wt t feature-login

# Create a worktree from a Jira issue
wt jira new PROJ-472

# Create from Jira with a custom branch name, opened in tmux
wt jira new -t -b my-branch PROJ-472

# Check the status of a Jira issue (auto-detects from current branch)
wt jira status

# Sync Jira status from GitHub PR state (dry run)
wt jira status sync -n

# Bootstrap a Jira status config
wt jira config --init
```

## Interactive TUI

Running `wt` with no arguments opens a full-screen TUI.

### Worktree list

| Key | Action |
|-----|--------|
| `enter` | Open shell in selected worktree |
| `t` | Open in tmux session |
| `n` | Create new worktree (select branch) |
| `d` | Delete selected worktree |
| `/` | Filter worktrees |
| `q` | Quit |

### Branch selection

| Key | Action |
|-----|--------|
| `enter` | Use selected branch as-is |
| `c` | Create new branch from selected branch |
| `esc` | Back to worktree list |
| `/` | Filter branches |

When creating a new branch with `c`, you'll be prompted to enter a name, then
confirm before proceeding to the config copy prompts.

## Jira Configuration

`wt` looks for status mappings in two places (repo-level overrides global):

- **Global:** `~/.config/wt/config.json`
- **Repository:** `.wt.json` at the repo root

Run `wt jira config --init` to interactively bootstrap a config. The template
maps symbolic statuses to your Jira workflow's actual status names:

```json
{
  "jira": {
    "status": {
      "default": {
        "working": "In Progress",
        "review": "In Review",
        "testing": "Testing",
        "done": "Done"
      },
      "types": {}
    }
  }
}
```

The `types` object lets you override mappings for specific issue types when
your Jira workflows differ between, say, bugs and stories.

**Required environment variables** for Jira integration:

| Variable | Description |
|----------|-------------|
| `JIRA_URL` | Base URL of your Jira instance |
| `JIRA_USER` | Your Jira username |
| `JIRA_TOKEN` | Your Jira API token |
