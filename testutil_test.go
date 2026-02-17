package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// setupTestRepo creates a new git repository with an initial commit.
// Returns the path to the repository.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustRunCmd(t, dir, "git", "init", "-b", "main")
	mustRunCmd(t, dir, "git", "config", "user.email", "test@example.com")
	mustRunCmd(t, dir, "git", "config", "user.name", "Test")
	mustRunCmd(t, dir, "git", "config", "commit.gpgsign", "false")
	mustWriteFile(t, filepath.Join(dir, "file.txt"), "data")
	mustRunCmd(t, dir, "git", "add", ".")
	mustRunCmd(t, dir, "git", "commit", "-m", "init")
	return dir
}

// setupTestRepoWithBranches creates a git repo with multiple branches.
// Each branch has a unique commit to differentiate commit times.
func setupTestRepoWithBranches(t *testing.T, branches []string) string {
	t.Helper()
	repo := setupTestRepo(t)
	for _, branch := range branches {
		mustRunCmd(t, repo, "git", "branch", branch)
	}
	return repo
}

// setupTestWorktree creates a worktree for the given branch.
// Returns the path to the new worktree.
func setupTestWorktree(t *testing.T, repo, branch string) string {
	t.Helper()
	wtDir := repo + "-worktrees"
	wtPath := filepath.Join(wtDir, branch)
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Check if branch exists
	cmd := exec.Command("git", "-C", repo, "show-ref", "--verify", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
		// Branch doesn't exist, create with -b
		mustRunCmd(t, repo, "git", "worktree", "add", "-b", branch, wtPath)
	} else {
		mustRunCmd(t, repo, "git", "worktree", "add", wtPath, branch)
	}
	return wtPath
}

// mustRunCmd runs a command and fails the test if it errors.
func mustRunCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v (%s)", name, args, err, string(out))
	}
}

// mustWriteFile writes content to a file and fails the test if it errors.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// withDir temporarily changes to a directory for the duration of the test.
// Returns a cleanup function to restore the original directory.
func withDir(t *testing.T, dir string) func() {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	return func() { _ = os.Chdir(oldWd) }
}

type stubProgram struct {
	model tea.Model
	err   error
}

func (s stubProgram) Run() (tea.Model, error) {
	return s.model, s.err
}

type fakeDirEntry struct {
	name    string
	isDir   bool
	infoErr error
	mode    fs.FileMode
}

func (f fakeDirEntry) Name() string { return f.name }

func (f fakeDirEntry) IsDir() bool { return f.isDir }

func (f fakeDirEntry) Type() fs.FileMode { return f.mode }

func (f fakeDirEntry) Info() (fs.FileInfo, error) { return fakeFileInfo{mode: f.mode}, f.infoErr }

type fakeFileInfo struct {
	mode fs.FileMode
}

func (f fakeFileInfo) Name() string { return "file" }

func (f fakeFileInfo) Size() int64 { return 0 }

func (f fakeFileInfo) Mode() fs.FileMode { return f.mode }

func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }

func (f fakeFileInfo) IsDir() bool { return false }

func (f fakeFileInfo) Sys() any { return nil }

func cmdWithOutput(out string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", "printf '%s' \"$WT_OUT\"")
	cmd.Env = append(os.Environ(), "WT_OUT="+out)
	return cmd
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
