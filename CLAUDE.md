# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`wt` is a Go CLI tool for managing git worktrees with both CLI commands and an interactive TUI (BubbleTea framework). Worktrees are created under `${repo}-worktrees/` directories.

## Build & Test Commands

```bash
go build ./...                                    # Build
go test ./...                                     # Run all tests
go test ./... -coverprofile=/tmp/wt.cover         # Run tests with coverage (target: 100%)
go test -run TestName                             # Run single test
go test -run TestName -v                          # Run single test with verbose output
go tool cover -html=/tmp/wt.cover                 # View coverage report
```

## Architecture

Single-file architecture: `main.go` (implementation) and `main_test.go` (127 tests).

**CLI Commands:**
- `wt` (no args) - Opens interactive TUI
- `wt new <branch>` - Creates new worktree with config copy options
- `wt list` - Lists all worktrees
- `wt go <name>` - Opens shell in a worktree

**TUI State Machine** (`tuiState` type):
- States: `tuiStateList`, `tuiStateNewBranch`, `tuiStatePromptConfig`, `tuiStatePromptLibs`, `tuiStateConfirmDelete`, `tuiStateBusy`
- Implements Elm-style Update/View pattern via BubbleTea

**Dependency Injection for Testing:**
Global function variables (`execCommand`, `newProgram`, `osMkdirAll`, etc.) allow tests to stub system calls without modifying core logic.

**Default Copy Items** (line 23-24):
- Config files: `.env`, `AGENTS.md`, `CLAUDE.md` (copied by default)
- Lib directories: `node_modules` (only with `--copy-libs`)

## Coding Conventions

- Standard Go formatting (`gofmt`), tabs for indentation
- Keep functions small and purpose-driven
- Explicit error handling throughout
- Naming: lowerCamelCase for locals, UpperCamelCase for exports

## Testing

- 100% statement coverage target
- Tests use function variable reassignment to stub system calls
- Test new branches and error paths
