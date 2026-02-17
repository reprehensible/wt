package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreePath(t *testing.T) {
	got := worktreePath("/repo", "feature/one")
	want := filepath.Join("/repo-worktrees", "feature", "one")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestOrderByRecentCommitWorktrees(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	path := filepath.Join(t.TempDir(), "wt")
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 1 && args[0] == "-C" && args[1] == path {
			return cmdWithOutput("300")
		}
		ref := args[len(args)-1]
		if ref == "dev" {
			return cmdWithOutput("400")
		}
		return cmdWithOutput("0")
	}

	items := []string{path, "dev"}
	got := orderByRecentCommit(items, "/repo", "worktrees")
	want := []string{"dev", path}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestOrderByRecentCommitDefault(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("123")
	}

	items := []string{"one", "two"}
	got := orderByRecentCommit(items, "/repo", "other")
	if fmt.Sprintf("%v", got) != "[one two]" {
		t.Fatalf("unexpected order %v", got)
	}
}

func TestGitCommitTimeError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	if got := gitCommitTime("/repo", "main"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestGitCommitTimeParseError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("notanint")
	}

	if got := gitCommitTime("/repo", "main"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestGitCommitTimePathError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	if got := gitCommitTimePath("/repo"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestGitCommitTimePathSuccess(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("456")
	}
	if got := gitCommitTimePath("/repo"); got != 456 {
		t.Fatalf("expected 456, got %d", got)
	}
}

func TestGitCommitTimePathParseError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("notanint")
	}
	if got := gitCommitTimePath("/repo"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestRunGitOutput(t *testing.T) {
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "printf 'ok'")
	}
	if out, err := runGitOutput("/repo", "status"); err != nil || out != "ok" {
		t.Fatalf("expected ok, got %q err %v", out, err)
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo boom >&2; exit 1")
	}
	if _, err := runGitOutput("/repo", "status"); err == nil {
		t.Fatalf("expected error")
	}

	execCommand = oldExec
}

func TestRunGit(t *testing.T) {
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 0")
	}
	defer func() { execCommand = oldExec }()

	if err := runGit("/repo", "status"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestGitHelpersWithRepo(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	root, err := gitRepoRoot()
	if err != nil || root != repo {
		t.Fatalf("expected repo root %q, got %q err %v", repo, root, err)
	}

	mustRunCmd(t, repo, "git", "branch", "dev")

	branches, err := gitBranches(repo)
	if err != nil || !contains(branches, "dev") {
		t.Fatalf("expected dev branch, got %v err %v", branches, err)
	}

	ok, err := gitBranchExists(repo, "dev")
	if err != nil || !ok {
		t.Fatalf("expected dev to exist, got %v err %v", ok, err)
	}

	ok, err = gitBranchExists(repo, "missing-branch")
	if err != nil || ok {
		t.Fatalf("expected missing branch false, got %v err %v", ok, err)
	}

	if ts := gitCommitTime(repo, "main"); ts == 0 {
		t.Fatalf("expected commit time")
	}
}

func TestGitRepoRootError(t *testing.T) {
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	defer func() { execCommand = oldExec }()

	if _, err := gitRepoRoot(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGitBranchesErrorAndBlanks(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("main\n\n dev\n")
	}
	branches, err := gitBranches("/repo")
	if err != nil || !contains(branches, "main") || !contains(branches, "dev") {
		t.Fatalf("unexpected branches %v err %v", branches, err)
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	if _, err := gitBranches("/repo"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGitBranchExistsError(t *testing.T) {
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("does-not-exist")
	}
	defer func() { execCommand = oldExec }()

	if _, err := gitBranchExists("/repo", "dev"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGitWorktreesParse(t *testing.T) {
	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"weirdline",
		"",
		"worktree /repo-wt",
		"",
	}, "\n")

	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput(out)
	}
	defer func() { execCommand = oldExec }()

	wts, err := gitWorktrees("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 2 || wts[0].Branch != "main" || wts[1].Path != "/repo-wt" {
		t.Fatalf("unexpected worktrees: %v", wts)
	}
}

func TestGitWorktreesFinalAppend(t *testing.T) {
	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
	}, "\n")

	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput(out)
	}
	defer func() { execCommand = oldExec }()

	wts, err := gitWorktrees("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wts) != 1 || wts[0].Branch != "main" {
		t.Fatalf("unexpected worktrees: %v", wts)
	}
}

func TestGitWorktreeClean(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("")
	}
	clean, err := gitWorktreeClean("/repo")
	if err != nil || !clean {
		t.Fatalf("expected clean true, got %v err %v", clean, err)
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput(" M file")
	}
	clean, err = gitWorktreeClean("/repo")
	if err != nil || clean {
		t.Fatalf("expected clean false, got %v err %v", clean, err)
	}
}

func TestGitWorktreeCleanError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	if _, err := gitWorktreeClean("/repo"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGitMainWorktreeError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	if _, err := gitMainWorktree("/repo"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGitMainWorktreeEmpty(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("")
	}
	_, err := gitMainWorktree("/repo")
	if err == nil || err.Error() != "no worktrees found" {
		t.Fatalf("expected 'no worktrees found' error, got %v", err)
	}
}
