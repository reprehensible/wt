package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationNewCmdWithRealGit(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	// Create a branch
	mustRunCmd(t, repo, "git", "branch", "feature")

	// Create config files to copy
	mustWriteFile(t, filepath.Join(repo, ".env"), "SECRET=123")
	mustWriteFile(t, filepath.Join(repo, "CLAUDE.md"), "# Instructions")

	oldOut := stdout
	defer func() { stdout = oldOut }()
	var buf bytes.Buffer
	stdout = &buf

	newCmd([]string{"-C", "-L", "feature"})

	wtPath := worktreePath(repo, "feature")
	if !strings.Contains(buf.String(), wtPath) {
		t.Fatalf("expected worktree path in output, got %q", buf.String())
	}

	// Verify worktree was created
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree not created: %v", err)
	}

	// Verify .env was not copied (because -C disables config copy)
	if _, err := os.Stat(filepath.Join(wtPath, ".env")); !os.IsNotExist(err) {
		t.Fatalf("expected .env to not be copied with -C flag")
	}
}

func TestIntegrationNewCmdCopiesConfig(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	// Create config files to copy
	mustWriteFile(t, filepath.Join(repo, ".env"), "SECRET=123")
	mustWriteFile(t, filepath.Join(repo, "AGENTS.md"), "# Agents")

	oldOut := stdout
	defer func() { stdout = oldOut }()
	var buf bytes.Buffer
	stdout = &buf

	newCmd([]string{"feature2"})

	wtPath := worktreePath(repo, "feature2")

	// Verify config files were copied
	content, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatalf("expected .env to be copied: %v", err)
	}
	if string(content) != "SECRET=123" {
		t.Fatalf("unexpected .env content: %q", content)
	}

	content, err = os.ReadFile(filepath.Join(wtPath, "AGENTS.md"))
	if err != nil {
		t.Fatalf("expected AGENTS.md to be copied: %v", err)
	}
	if string(content) != "# Agents" {
		t.Fatalf("unexpected AGENTS.md content: %q", content)
	}
}

func TestIntegrationListCmdWithRealGit(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	oldOut := stdout
	defer func() { stdout = oldOut }()
	var buf bytes.Buffer
	stdout = &buf

	listCmd(nil)

	output := buf.String()
	// Should list the main worktree
	if !strings.Contains(output, "main") && !strings.Contains(output, repo) {
		t.Fatalf("expected worktree listing, got %q", output)
	}
}

func TestIntegrationListCmdWithWorktrees(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	// Create a worktree
	wtPath := setupTestWorktree(t, repo, "dev")

	oldOut := stdout
	defer func() { stdout = oldOut }()
	var buf bytes.Buffer
	stdout = &buf

	listCmd(nil)

	output := buf.String()
	// Should list both worktrees
	if !strings.Contains(output, "main") {
		t.Fatalf("expected main branch in output, got %q", output)
	}
	if !strings.Contains(output, "dev") || !strings.Contains(output, wtPath) {
		t.Fatalf("expected dev worktree in output, got %q", output)
	}
}

func TestIntegrationGitBranchesWithRealGit(t *testing.T) {
	repo := setupTestRepoWithBranches(t, []string{"dev", "feature"})
	defer withDir(t, repo)()

	branches, err := gitBranches(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(branches, "main") {
		t.Fatalf("expected main branch, got %v", branches)
	}
	if !contains(branches, "dev") {
		t.Fatalf("expected dev branch, got %v", branches)
	}
	if !contains(branches, "feature") {
		t.Fatalf("expected feature branch, got %v", branches)
	}
}

func TestIntegrationGitWorktreesWithRealGit(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	wts, err := gitWorktrees(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Path != repo {
		t.Fatalf("expected path %q, got %q", repo, wts[0].Path)
	}
	if wts[0].Branch != "main" && wts[0].Branch != "master" {
		t.Fatalf("expected main/master branch, got %q", wts[0].Branch)
	}
}

func TestIntegrationGitWorktreeCleanWithRealGit(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	// Initially clean
	clean, err := gitWorktreeClean(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clean {
		t.Fatalf("expected clean worktree")
	}

	// Make it dirty
	mustWriteFile(t, filepath.Join(repo, "dirty.txt"), "dirty")

	clean, err = gitWorktreeClean(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean {
		t.Fatalf("expected dirty worktree")
	}
}

func TestIntegrationGitCommitTimeWithRealGit(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	ts := gitCommitTime(repo, "main")
	if ts == 0 {
		// Try master if main doesn't exist
		ts = gitCommitTime(repo, "master")
	}
	if ts == 0 {
		t.Fatalf("expected non-zero commit time")
	}

	// Verify it's a reasonable timestamp (after year 2020)
	if ts < 1577836800 {
		t.Fatalf("commit time %d seems too old", ts)
	}
}

func TestIntegrationOrderByRecentCommitWithRealGit(t *testing.T) {
	repo := setupTestRepo(t)
	defer withDir(t, repo)()

	// Create branch from current commit
	mustRunCmd(t, repo, "git", "branch", "older")

	// Make a new commit on main with a later date to ensure ordering
	mustWriteFile(t, filepath.Join(repo, "new.txt"), "new")
	mustRunCmd(t, repo, "git", "add", ".")
	// Use GIT_COMMITTER_DATE to ensure different timestamps
	cmd := exec.Command("git", "-c", "commit.gpgsign=false", "commit", "-m", "newer commit")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2099-01-01T00:00:00")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}

	branches := []string{"older", "main"}
	ordered := orderByRecentCommit(branches, repo, "branches")

	// main should be first (more recent due to 2099 date)
	if ordered[0] != "main" {
		t.Fatalf("expected main first, got %v", ordered)
	}
}
