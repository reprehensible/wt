package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCmdFromFlag(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			gotArgs = args
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	newCmd([]string{"--from", "develop", "feature"})

	foundB := false
	foundBase := false
	for _, arg := range gotArgs {
		if arg == "-b" {
			foundB = true
		}
		if arg == "develop" {
			foundBase = true
		}
	}
	if !foundB || !foundBase {
		t.Fatalf("expected -b and develop in args, got %v", gotArgs)
	}
}

func TestNewCmdFromFlagError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"--from", "develop", "feature"})
}

func TestListCmd(t *testing.T) {
	oldExec := execCommand
	oldStdout := stdout
	defer func() {
		execCommand = oldExec
		stdout = oldStdout
	}()

	// Include a worktree without a branch (detached HEAD case)
	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
		"worktree /repo-wt",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	var buf bytes.Buffer
	stdout = &buf
	listCmd(nil)

	// Verifies branch+path output format
	if !strings.Contains(buf.String(), "main\t/repo") {
		t.Fatalf("expected branch output, got %q", buf.String())
	}
	// Verifies path-only output for worktree without branch
	if !strings.Contains(buf.String(), "/repo-wt") {
		t.Fatalf("expected worktree output, got %q", buf.String())
	}
}

func TestListCmdArgs(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	listCmd([]string{"extra"})
}

func TestListCmdRepoRootError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	listCmd(nil)
}

func TestListCmdWorktreesError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	listCmd(nil)
}

func TestNewCmdBranchRequired(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd(nil)
}

func TestNewCmdMkdirError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	oldMkdir := osMkdirAll
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		osMkdirAll = oldMkdir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("mkdir fail")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestNewCmdCopies(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("env"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("agents"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	newCmd([]string{"main"})

	wtPath := worktreePath(repo, "main")
	if _, err := os.Stat(filepath.Join(wtPath, ".env")); err != nil {
		t.Fatalf("expected .env copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "node_modules", "a.txt")); err == nil {
		t.Fatalf("expected no node_modules copy by default")
	}
}

func TestNewCmdCopyLibs(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	newCmd([]string{"--copy-libs", "libs"})

	wtPath := worktreePath(repo, "libs")
	if _, err := os.Stat(filepath.Join(wtPath, "node_modules", "a.txt")); err != nil {
		t.Fatalf("expected node_modules copy: %v", err)
	}
}

func TestNewCmdCopyLibsError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	oldStat := osStat
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		osStat = oldStat
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osStat = func(name string) (fs.FileInfo, error) {
		return nil, errors.New("stat fail")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"--no-copy-config", "--copy-libs", "libs"})
}

func TestNewCmdRepoRootError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestNewCmdMainWorktreeError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestNewCmdBranchExistsError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("does-not-exist")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestNewCmdWorktreeAddError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestNewCmdWorktreeAddNewBranchError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1") // branch doesn't exist
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"new-branch"})
}

func TestCreateWorktreeBaseBranchError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "feature",
		baseBranch:    "develop",
		copyConfig:    false,
		copyLibs:      false,
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewCmdCopyError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	oldStat := osStat
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		osStat = oldStat
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osStat = func(name string) (fs.FileInfo, error) {
		return nil, errors.New("stat fail")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestNewCmdCopyMatchingFilesError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	oldWalk := filepathWalkDir
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		filepathWalkDir = oldWalk
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return errors.New("walk fail")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	newCmd([]string{"main"})
}

func TestGoCmdSuccess(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldEnv := os.Getenv("SHELL")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("SHELL", oldEnv)
	}()

	out := strings.Join([]string{
		"worktree " + repo,
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "/bin/true" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	_ = os.Setenv("SHELL", "/bin/true")
	goCmd([]string{"main"})
}

func TestGoCmdRequiresArg(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	goCmd(nil)
}

func TestGoCmdNoWorktrees(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	goCmd([]string{"main"})
}

func TestGoCmdNotFoundAndDefaultShell(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldEnv := os.Getenv("SHELL")
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		_ = os.Setenv("SHELL", oldEnv)
	}()

	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "/bin/sh" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	_ = os.Unsetenv("SHELL")
	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	goCmd([]string{"unknown"})
}

func TestGoCmdDefaultShellSuccess(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldEnv := os.Getenv("SHELL")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("SHELL", oldEnv)
	}()

	out := strings.Join([]string{
		"worktree " + repo,
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "/bin/sh" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	_ = os.Unsetenv("SHELL")
	goCmd([]string{"main"})
}

func TestGoCmdWorktreesError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	goCmd([]string{"main"})
}

func TestGoCmdRepoRootError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	goCmd([]string{"main"})
}

func TestGoCmdMatchBaseAndPath(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldEnv := os.Getenv("SHELL")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("SHELL", oldEnv)
	}()

	out := strings.Join([]string{
		"worktree " + filepath.Join(repo, "alpha"),
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "/bin/true" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	_ = os.Setenv("SHELL", "/bin/true")
	if err := os.MkdirAll(filepath.Join(repo, "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	goCmd([]string{"alpha"})
	goCmd([]string{filepath.Join(repo, "alpha")})
}

func TestGoCmdRunError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldExit := exitFunc
	oldEnv := os.Getenv("SHELL")
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		_ = os.Setenv("SHELL", oldEnv)
	}()

	out := strings.Join([]string{
		"worktree " + repo,
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "/bin/true" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	_ = os.Setenv("SHELL", "/bin/true")
	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	goCmd([]string{"main"})
}

func TestDie(t *testing.T) {
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		exitFunc = oldExit
		stderr = oldErr
	}()

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "boom") {
			t.Fatalf("expected error output, got %q", buf.String())
		}
	}()

	die(errors.New("boom"))
}

func TestNewCmdCopiesFromMainWorktree(t *testing.T) {
	// Setup: main repo has .env, but we're running from a worktree that doesn't
	mainRepo := t.TempDir()
	worktreeDir := mainRepo + "-worktrees"
	existingWT := filepath.Join(worktreeDir, "feature1")
	if err := os.MkdirAll(existingWT, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// .env only exists in main repo, not in the existing worktree
	if err := os.WriteFile(filepath.Join(mainRepo, ".env"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		// Simulate running from inside existingWT worktree
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(existingWT)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			// First entry is always the main worktree
			out := fmt.Sprintf("worktree %s\nbranch refs/heads/main\n\nworktree %s\nbranch refs/heads/feature1\n", mainRepo, existingWT)
			return cmdWithOutput(out)
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	newCmd([]string{"feature2"})

	newWT := filepath.Join(worktreeDir, "feature2")
	if _, err := os.Stat(filepath.Join(newWT, ".env")); err != nil {
		t.Fatalf("expected .env to be copied from main worktree: %v", err)
	}
	// Verify content is from main repo
	content, err := os.ReadFile(filepath.Join(newWT, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(content) != "secret" {
		t.Fatalf("expected .env content 'secret', got %q", content)
	}
}

func TestNewCmdCopiesEnvFromSubdirectories(t *testing.T) {
	repo := t.TempDir()

	// Create .env files in root and subdirectories
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("ROOT"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "backend"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "backend", ".env"), []byte("BACKEND"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "services", "api"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "services", "api", ".env"), []byte("API"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	newCmd([]string{"feature"})

	wtPath := worktreePath(repo, "feature")

	// Check root .env
	content, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	if err != nil {
		t.Fatalf("expected root .env: %v", err)
	}
	if string(content) != "ROOT" {
		t.Fatalf("expected root .env content 'ROOT', got %q", content)
	}

	// Check backend/.env
	content, err = os.ReadFile(filepath.Join(wtPath, "backend", ".env"))
	if err != nil {
		t.Fatalf("expected backend/.env: %v", err)
	}
	if string(content) != "BACKEND" {
		t.Fatalf("expected backend/.env content 'BACKEND', got %q", content)
	}

	// Check services/api/.env
	content, err = os.ReadFile(filepath.Join(wtPath, "services", "api", ".env"))
	if err != nil {
		t.Fatalf("expected services/api/.env: %v", err)
	}
	if string(content) != "API" {
		t.Fatalf("expected services/api/.env content 'API', got %q", content)
	}
}

// Integration tests using real git repos

func TestTmuxCmdRequiresArg(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	tmuxCmd(nil)
}

func TestTmuxCmdWorktreeNotFound(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	tmuxCmd([]string{"nonexistent"})
}

func TestTmuxCmdNoWorktrees(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	tmuxCmd([]string{"main"})
}

func TestTmuxCmdRepoRootError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo fail; exit 1")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	tmuxCmd([]string{"main"})
}

func TestTmuxCmdWorktreesError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		// worktree list fails
		return exec.Command("sh", "-c", "echo fail; exit 1")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	tmuxCmd([]string{"main"})
}

func TestOpenTmuxNewSessionNotInTmux(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	var tmuxArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 1") // session doesn't exist
		}
		if name == "tmux" && len(args) > 0 && args[0] == "new-session" {
			tmuxArgs = append([]string{args[0]}, args[1:]...)
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tmuxArgs) == 0 {
		t.Fatal("expected tmux new-session to be called")
	}
	joined := strings.Join(tmuxArgs, " ")
	if !strings.Contains(joined, "-s feature") {
		t.Fatalf("expected session name 'feature', got args: %v", tmuxArgs)
	}
	if !strings.Contains(joined, "-c /repo/feature") {
		t.Fatalf("expected working dir '/repo/feature', got args: %v", tmuxArgs)
	}
	// Should NOT be detached (no -d) since not in tmux
	if strings.Contains(joined, "-d") {
		t.Fatalf("expected attached session (no -d), got args: %v", tmuxArgs)
	}
}

func TestOpenTmuxNewSessionInTmux(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	var calls []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 1") // session doesn't exist
		}
		if name == "tmux" {
			calls = append(calls, args[0])
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) < 2 {
		t.Fatalf("expected new-session and switch-client calls, got %v", calls)
	}
	if calls[0] != "new-session" {
		t.Fatalf("expected new-session first, got %v", calls)
	}
	if calls[1] != "switch-client" {
		t.Fatalf("expected switch-client second, got %v", calls)
	}
}

func TestOpenTmuxExistingSessionInTmux(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	var lastCall string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 0") // session exists
		}
		if name == "tmux" {
			lastCall = args[0]
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastCall != "switch-client" {
		t.Fatalf("expected switch-client, got %q", lastCall)
	}
}

func TestOpenTmuxExistingSessionNotInTmux(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	var lastCall string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 0") // session exists
		}
		if name == "tmux" {
			lastCall = args[0]
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastCall != "attach-session" {
		t.Fatalf("expected attach-session, got %q", lastCall)
	}
}

func TestOpenTmuxSessionName(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	var sessionName string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			for i, a := range args {
				if a == "-t" && i+1 < len(args) {
					sessionName = args[i+1]
				}
			}
			return exec.Command("sh", "-c", "exit 1")
		}
		if name == "tmux" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/home/user/repo-worktrees/my-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessionName != "my-feature" {
		t.Fatalf("expected session name 'my-feature', got %q", sessionName)
	}
}

func TestOpenTmuxNewSessionError(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if name == "tmux" && len(args) > 0 && args[0] == "new-session" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err == nil {
		t.Fatal("expected error from failed new-session")
	}
}

func TestOpenTmuxSwitchClientError(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 0") // session exists
		}
		if name == "tmux" && len(args) > 0 && args[0] == "switch-client" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err == nil {
		t.Fatal("expected error from failed switch-client")
	}
}

func TestOpenTmuxNewSessionInTmuxCreateError(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 1") // doesn't exist
		}
		if name == "tmux" && len(args) > 0 && args[0] == "new-session" {
			return exec.Command("sh", "-c", "exit 1") // create fails
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err == nil {
		t.Fatal("expected error from failed new-session in tmux")
	}
}

func TestTmuxCmdSuccess(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	tmuxCmd([]string{"main"})
}

func TestTmuxCmdMatchBaseAndPath(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	wtPath := filepath.Join(repo+"-worktrees", "feature")
	// Use no branch so base-path matching is the only match
	outNoBranch := strings.Join([]string{
		"worktree " + repo,
		"branch refs/heads/main",
		"",
		"worktree " + wtPath,
		"",
	}, "\n")
	outWithBranch := strings.Join([]string{
		"worktree " + repo,
		"branch refs/heads/main",
		"",
		"worktree " + wtPath,
		"branch refs/heads/feature",
		"",
	}, "\n")

	var matched bool
	makeExec := func(out string) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "tmux" {
				matched = true
				return exec.Command("sh", "-c", "exit 0")
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput(repo)
			}
			if len(args) >= 2 && args[0] == "worktree" {
				return cmdWithOutput(out)
			}
			return exec.Command("sh", "-c", "exit 0")
		}
	}

	// Match by branch name
	makeExec(outWithBranch)
	tmuxCmd([]string{"feature"})
	if !matched {
		t.Fatal("expected tmux to be called for branch match")
	}

	// Match by base path (no branch set, so only base path matches)
	matched = false
	makeExec(outNoBranch)
	tmuxCmd([]string{filepath.Base(wtPath)})
	if !matched {
		t.Fatal("expected tmux to be called for base path match")
	}

	// Match by full path
	matched = false
	makeExec(outNoBranch)
	tmuxCmd([]string{wtPath})
	if !matched {
		t.Fatal("expected tmux to be called for full path match")
	}
}

func TestOpenTmuxAttachError(t *testing.T) {
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" {
			return exec.Command("sh", "-c", "exit 0") // session exists
		}
		if name == "tmux" && len(args) > 0 && args[0] == "attach-session" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	err := openTmux("/repo/feature")
	if err == nil {
		t.Fatal("expected error from failed attach-session")
	}
}

func TestTmuxCmdOpenTmuxError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldEnv := os.Getenv("TMUX")
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		_ = os.Setenv("TMUX", oldEnv)
	}()

	_ = os.Unsetenv("TMUX")

	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
	}, "\n")

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" {
			return cmdWithOutput(out)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	tmuxCmd([]string{"main"})
}

// --- Jira tests ---
