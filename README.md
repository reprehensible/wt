# wt

[![Go](https://img.shields.io/github/go-mod/go-version/reprehensible/wt)](https://github.com/reprehensible/wt)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A CLI tool for managing git worktrees. Create, list, navigate, and delete
worktrees from the command line or an interactive TUI.

Worktrees are created under `<repo>-worktrees/` alongside your main checkout.

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
