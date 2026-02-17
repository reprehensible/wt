package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMainNoArgs(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldErr := stderr
	oldProgram := newProgram
	oldExec := execCommand
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		stderr = oldErr
		newProgram = oldProgram
		execCommand = oldExec
	}()

	os.Args = []string{"wt"}
	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput("worktree /repo\nbranch refs/heads/main\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	newProgram = func(model tea.Model, opts ...tea.ProgramOption) programRunner {
		return stubProgram{model: tuiModel{action: tuiAction{kind: tuiActionNone}}}
	}

	main()
}

func TestMainNoArgsGoActionError(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldProgram := newProgram
	oldExec := execCommand
	oldEnv := os.Getenv("SHELL")
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		newProgram = oldProgram
		execCommand = oldExec
		_ = os.Setenv("SHELL", oldEnv)
	}()

	os.Args = []string{"wt"}
	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	_ = os.Setenv("SHELL", "/bin/false")
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "/bin/false" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput("/repo")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput("worktree /repo\nbranch refs/heads/main\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	newProgram = func(model tea.Model, opts ...tea.ProgramOption) programRunner {
		return stubProgram{model: tuiModel{action: tuiAction{kind: tuiActionGo, path: "/repo"}}}
	}

	main()
}

func TestMainNoArgsRunTUIError(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldExec := execCommand
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		execCommand = oldExec
	}()

	os.Args = []string{"wt"}
	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	main()
}

func TestMainNoArgsGoActionSuccess(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldProgram := newProgram
	oldExec := execCommand
	oldEnv := os.Getenv("SHELL")
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		newProgram = oldProgram
		execCommand = oldExec
		_ = os.Setenv("SHELL", oldEnv)
	}()

	os.Args = []string{"wt"}
	exitFunc = func(code int) { panic(code) }
	_ = os.Setenv("SHELL", "/bin/true")
	repo := t.TempDir()

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
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	newProgram = func(model tea.Model, opts ...tea.ProgramOption) programRunner {
		return stubProgram{model: tuiModel{action: tuiAction{kind: tuiActionGo, path: repo}}}
	}

	main()
}

func TestMainNoArgsTmuxActionSuccess(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldProgram := newProgram
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		newProgram = oldProgram
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	os.Args = []string{"wt"}
	exitFunc = func(code int) { panic(code) }
	_ = os.Unsetenv("TMUX")
	repo := t.TempDir()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	newProgram = func(model tea.Model, opts ...tea.ProgramOption) programRunner {
		return stubProgram{model: tuiModel{action: tuiAction{kind: tuiActionTmux, path: repo}}}
	}

	main()
}

func TestMainNoArgsTmuxActionError(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldProgram := newProgram
	oldExec := execCommand
	oldEnv := os.Getenv("TMUX")
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		newProgram = oldProgram
		execCommand = oldExec
		_ = os.Setenv("TMUX", oldEnv)
	}()

	os.Args = []string{"wt"}
	exitFunc = func(code int) { panic(code) }
	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	_ = os.Unsetenv("TMUX")
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
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput("worktree /repo\nbranch refs/heads/main\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	newProgram = func(model tea.Model, opts ...tea.ProgramOption) programRunner {
		return stubProgram{model: tuiModel{action: tuiAction{kind: tuiActionTmux, path: "/repo"}}}
	}

	main()
}

func TestMainUnknownCommand(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
		stderr = oldErr
	}()

	os.Args = []string{"wt", "nope"}
	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) {
		panic(code)
	}

	defer func() {
		if r := recover(); r != 2 {
			t.Fatalf("expected exit 2, got %v", r)
		}
		if !strings.Contains(buf.String(), "unknown command") {
			t.Fatalf("expected unknown command output, got %q", buf.String())
		}
	}()

	main()
}

func TestMainHelp(t *testing.T) {
	oldArgs := os.Args
	oldErr := stderr
	defer func() {
		os.Args = oldArgs
		stderr = oldErr
	}()

	os.Args = []string{"wt", "help"}
	var buf bytes.Buffer
	stderr = &buf

	main()

	if !strings.Contains(buf.String(), "usage: wt") {
		t.Fatalf("expected usage output, got %q", buf.String())
	}
}

func TestMainDispatch(t *testing.T) {
	oldArgs := os.Args
	oldNew := newCmdFn
	oldList := listCmdFn
	oldGo := goCmdFn
	oldTmux := tmuxCmdFn
	oldJira := jiraCmdFn
	defer func() {
		os.Args = oldArgs
		newCmdFn = oldNew
		listCmdFn = oldList
		goCmdFn = oldGo
		tmuxCmdFn = oldTmux
		jiraCmdFn = oldJira
	}()

	calls := map[string]bool{}
	newCmdFn = func(args []string) { calls["new"] = true }
	listCmdFn = func(args []string) { calls["list"] = true }
	goCmdFn = func(args []string) { calls["go"] = true }
	tmuxCmdFn = func(args []string) { calls["t"] = true }
	jiraCmdFn = func(args []string) { calls["jira"] = true }

	for _, cmd := range []string{"new", "list", "go", "t", "jira"} {
		os.Args = []string{"wt", cmd}
		main()
		if !calls[cmd] {
			t.Fatalf("expected %s to be called", cmd)
		}
	}
}
