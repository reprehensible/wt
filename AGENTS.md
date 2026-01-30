# Repository Guidelines

## Project Structure & Module Organization
- `main.go` contains the CLI implementation for `wt`.
- `main_test.go` holds unit tests (including error-path coverage).
- `go.mod` and `go.sum` define module dependencies.
- `README.md` provides the short project description.

## Build, Test, and Development Commands
- `go build ./...` builds the binary for local use.
- `go test ./...` runs the full test suite.
- `go test ./... -coverprofile=/tmp/wt.cover` runs tests with coverage output (current target is 100%).

## Coding Style & Naming Conventions
- Follow standard Go formatting (`gofmt`); use tabs for indentation.
- Keep functions small and purpose-driven; prefer explicit error handling.
- Naming: lowerCamelCase for locals, UpperCamelCase for exported types, and concise but clear identifiers.

## Testing Guidelines
- Tests use Go’s standard `testing` package.
- Coverage is expected to be 100% of statements; add tests for new branches and error paths.
- Test functions should follow `TestXxx` naming and live in `*_test.go` files.

## Commit & Pull Request Guidelines
- Commit messages in history use short, sentence-case statements with periods (e.g., "Init commit.").
- PRs should include a brief description of intent, key changes, and any testing performed.
- Link relevant issues if they exist; include CLI output or screenshots only when behavior changes are user-visible.

## Configuration & Tips
- Worktree lists are ordered by most recent commit time per branch/worktree.
- Running `wt` with no arguments opens the TUI for browsing, creating, and deleting worktrees.
- Config files (e.g., `.env`, `AGENTS.md`, `CLAUDE.md`) copy by default; libs (e.g., `node_modules`) only copy with `--copy-libs`.
- Worktrees are created under `${repo}-worktrees/` and named after branches.
