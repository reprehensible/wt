package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

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

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return f.isDir }
func (f fakeDirEntry) Type() fs.FileMode          { return f.mode }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return fakeFileInfo{mode: f.mode}, f.infoErr }

type fakeFileInfo struct {
	mode fs.FileMode
}

func (f fakeFileInfo) Name() string       { return "file" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

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

func TestRunTUISuccess(t *testing.T) {
	oldProgram := newProgram
	oldExec := execCommand
	defer func() {
		newProgram = oldProgram
		execCommand = oldExec
	}()

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
		return stubProgram{model: tuiModel{action: tuiAction{kind: tuiActionGo, path: "/repo"}}}
	}

	action, err := runTUI()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.kind != tuiActionGo || action.path != "/repo" {
		t.Fatalf("unexpected action: %+v", action)
	}
}

func TestDefaultNewProgram(t *testing.T) {
	prog := newProgram(tuiModel{}, tea.WithAltScreen())
	if prog == nil {
		t.Fatalf("expected program")
	}
}

func TestRunTUIError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	if _, err := runTUI(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunTUIProgramError(t *testing.T) {
	oldExec := execCommand
	oldProgram := newProgram
	defer func() {
		execCommand = oldExec
		newProgram = oldProgram
	}()

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
		return stubProgram{err: errors.New("boom")}
	}

	if _, err := runTUI(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunTUIModelError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

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

	if _, err := runTUI(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewTUIModelError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	if _, err := newTUIModel("/repo"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewTUIModelSuccess(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
	}, "\n")
	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput(out)
	}

	model, err := newTUIModel("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(model.list.Items()) != 1 {
		t.Fatalf("expected list items")
	}
}

func TestNewTUIModelNoWorktrees(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput("")
	}

	_, err := newTUIModel("/repo")
	if err == nil || err.Error() != "no worktrees found" {
		t.Fatalf("expected 'no worktrees found' error, got %v", err)
	}
}

func TestTUIInit(t *testing.T) {
	model := tuiModel{}
	if model.Init() != nil {
		t.Fatalf("expected nil init")
	}
}

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

func TestExactMatchFilter(t *testing.T) {
	targets := []string{"main", "feature", "hotfix"}
	if got := exactMatchFilter("", targets); len(got) != 3 {
		t.Fatalf("expected all targets, got %d", len(got))
	}

	got := exactMatchFilter("ea", targets)
	if len(got) != 1 || got[0].Index != 1 {
		t.Fatalf("expected feature match, got %v", got)
	}
	if len(got[0].MatchedIndexes) != 2 {
		t.Fatalf("expected matched indexes, got %v", got[0].MatchedIndexes)
	}

	got = exactMatchFilter("zzz", targets)
	if len(got) != 0 {
		t.Fatalf("expected no matches, got %v", got)
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

func TestWorktreeItem(t *testing.T) {
	item := worktreeItem{branch: "main", path: "/repo"}
	if item.Title() == "" || item.FilterValue() == "" {
		t.Fatalf("expected title/filter value")
	}
	item = worktreeItem{branch: "main", path: "/repo", display: "main  /repo"}
	if item.Title() != "main  /repo" {
		t.Fatalf("unexpected title %q", item.Title())
	}
	item = worktreeItem{branch: "main", path: "/repo", display: ""}
	if item.Title() != "/repo" {
		t.Fatalf("unexpected title %q", item.Title())
	}
	item = worktreeItem{path: "/repo"}
	if item.Title() != "/repo" {
		t.Fatalf("unexpected title %q", item.Title())
	}
	if item.FilterValue() != "/repo" {
		t.Fatalf("unexpected filter value %q", item.FilterValue())
	}
}

func TestBranchItem(t *testing.T) {
	item := branchItem("dev")
	if item.Title() != "dev" {
		t.Fatalf("unexpected title %q", item.Title())
	}
	if item.Description() != "" || item.FilterValue() != "dev" {
		t.Fatalf("unexpected branch item fields")
	}
}

func TestBuildWorktreeItems(t *testing.T) {
	items, _ := buildWorktreeItems([]worktree{
		{Branch: "main", Path: "/repo"},
		{Path: "/repo-other"},
	})
	if len(items) != 2 {
		t.Fatalf("expected 2 items")
	}
	wt, ok := items[0].(worktreeItem)
	if !ok || wt.display == "" {
		t.Fatalf("expected display string")
	}
}

func TestDenseDelegateRender(t *testing.T) {
	delegate := denseDelegate{DefaultDelegate: list.NewDefaultDelegate()}
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	items := []list.Item{worktreeItem{branch: "main", path: "/repo"}}
	model := list.New(items, delegate, 0, 0)
	model.SetSize(20, 5)
	model.SetFilteringEnabled(true)
	model.SetFilterText("")
	model.SetFilterState(list.Filtering)

	var buf bytes.Buffer
	delegate.Render(&buf, model, 0, items[0])
	if buf.Len() == 0 {
		t.Fatalf("expected rendered output")
	}
}

func TestDenseDelegateRenderNonDefaultItem(t *testing.T) {
	delegate := denseDelegate{DefaultDelegate: list.NewDefaultDelegate()}
	var buf bytes.Buffer
	delegate.Render(&buf, list.Model{}, 0, list.Item(nil))
	if buf.Len() != 0 {
		t.Fatalf("expected empty output for non-default item")
	}
}

func TestDenseDelegateRenderZeroWidth(t *testing.T) {
	delegate := denseDelegate{DefaultDelegate: list.NewDefaultDelegate()}
	items := []list.Item{worktreeItem{branch: "main", path: "/repo"}}
	model := list.New(items, delegate, 0, 0)
	model.SetSize(0, 5)

	var buf bytes.Buffer
	delegate.Render(&buf, model, 0, items[0])
	if buf.Len() != 0 {
		t.Fatalf("expected empty output for zero width")
	}
}

type descItem struct {
	title string
	desc  string
}

func (d descItem) Title() string       { return d.title }
func (d descItem) Description() string { return d.desc }
func (d descItem) FilterValue() string { return d.title }

func TestDenseDelegateRenderDescriptionFiltered(t *testing.T) {
	delegate := denseDelegate{DefaultDelegate: list.NewDefaultDelegate()}
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	delegate.SetSpacing(0)

	items := []list.Item{
		descItem{title: "alpha", desc: "line1\nline2"},
		descItem{title: "beta", desc: "line1\nline2"},
	}
	model := list.New(items, delegate, 0, 0)
	model.SetSize(20, 5)
	model.Filter = exactMatchFilter
	model.SetFilterText("a")
	model.SetFilterState(list.FilterApplied)

	var buf bytes.Buffer
	delegate.Render(&buf, model, 1, items[1])
	if buf.Len() == 0 {
		t.Fatalf("expected rendered output")
	}
}

func TestNewListModel(t *testing.T) {
	items := []list.Item{worktreeItem{branch: "main", path: "/repo"}}
	model := newListModel("Worktrees", items)
	if model.FilterState() != list.Unfiltered {
		t.Fatalf("expected unfiltered state, got %v", model.FilterState())
	}
}

func TestTUIListCommandsBlockedDuringFilter(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	// Enter filter mode by sending '/'
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(tuiModel)
	if updated.list.FilterState() != list.Filtering {
		t.Fatalf("expected filtering state, got %v", updated.list.FilterState())
	}
	// 'n' during filter should type into filter, not trigger new command
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated = next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state, got %v", updated.state)
	}
	if updated.list.FilterValue() != "n" {
		t.Fatalf("expected filter value 'n', got %q", updated.list.FilterValue())
	}
}

func TestSelectedWorktree(t *testing.T) {
	model := newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}})
	item := selectedWorktree(model)
	if item.path != "/repo" {
		t.Fatalf("unexpected selected path %q", item.path)
	}

	model = newListModel("Worktrees", []list.Item{branchItem("main")})
	item = selectedWorktree(model)
	if item.path != "" {
		t.Fatalf("expected empty selection")
	}
}

func TestPromptView(t *testing.T) {
	out := promptView("Copy config files?", true, "status")
	if !strings.Contains(out, "Copy config files?") || !strings.Contains(out, "status") {
		t.Fatalf("unexpected prompt output: %q", out)
	}
}

func TestWithStatusEmpty(t *testing.T) {
	out := withStatus("body", "")
	if out != "body" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestWithFooter(t *testing.T) {
	out := withFooter("body", "footer", "status")
	if !strings.Contains(out, "body") || !strings.Contains(out, "footer") || !strings.Contains(out, "status") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestFooters(t *testing.T) {
	if listFooter() == "" || branchFooter() == "" {
		t.Fatalf("expected footers")
	}
}

func TestRenderFramed(t *testing.T) {
	out := renderFramed("content", "help", "status", 40)
	if !strings.Contains(out, "content") || !strings.Contains(out, "help") || !strings.Contains(out, "status") {
		t.Fatalf("unexpected output: %q", out)
	}
	out = renderFramed("content", "", "", 0)
	if !strings.Contains(out, "content") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestColumnHeader(t *testing.T) {
	out := columnHeader(10)
	if !strings.Contains(out, "Branch") || !strings.Contains(out, "Path") {
		t.Fatalf("unexpected header: %q", out)
	}
	out = columnHeader(2)
	if !strings.Contains(out, "Branch") {
		t.Fatalf("expected Branch padded to minimum: %q", out)
	}
}

func TestListContent(t *testing.T) {
	model := tuiModel{
		list:         newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
		maxBranchLen: 4,
	}
	out := model.listContent()
	if !strings.Contains(out, "Worktrees") || !strings.Contains(out, "Branch") {
		t.Fatalf("expected title and header: %q", out)
	}
}

func TestTUIListEnterGo(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.action.kind != tuiActionGo || updated.action.path != "/repo" {
		t.Fatalf("expected go action, got %+v", updated.action)
	}
}

func TestTUIListEnterNoSelection(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{branchItem("main")}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.action.kind != tuiActionNone {
		t.Fatalf("expected no action")
	}
}

func TestTUIListTmux(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updated := next.(tuiModel)
	if updated.action.kind != tuiActionTmux || updated.action.path != "/repo" {
		t.Fatalf("expected tmux action with path /repo, got %+v", updated.action)
	}
}

func TestTUIListTmuxNoSelection(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{branchItem("main")}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updated := next.(tuiModel)
	if updated.action.kind != tuiActionNone {
		t.Fatalf("expected no action")
	}
}

func TestTUIListRemoveError(t *testing.T) {
	model := tuiModel{
		state:         tuiStateConfirmDelete,
		repoRoot:      "/repo",
		pendingDelete: worktreeItem{branch: "main", path: "/repo"},
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	updated := next.(tuiModel)
	next, _ = updated.Update(deleteResultMsg{err: errors.New("boom")})
	updated = next.(tuiModel)
	if updated.status == "" {
		t.Fatalf("expected status error")
	}
}

func TestTUIListNavigation(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}, worktreeItem{branch: "dev", path: "/repo-dev"}}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := next.(tuiModel)
	if updated.list.Index() != 1 {
		t.Fatalf("expected index 1, got %d", updated.list.Index())
	}
}

func TestTUIListDeleteNoSelection(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{branchItem("main")}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated := next.(tuiModel)
	if updated.status != "" {
		t.Fatalf("expected no status")
	}
}

func TestTUIListUpdateFilterInput(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	// Enter filter mode first
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(tuiModel)
	// Now type into filter
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated = next.(tuiModel)
	if updated.list.FilterValue() != "x" {
		t.Fatalf("expected filter value 'x', got %q", updated.list.FilterValue())
	}
}

func TestTUIBranchFlow(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "branch" {
			return cmdWithOutput("main\nfeature")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:    tuiStateList,
		repoRoot: repo,
		list:     newListModel("Worktrees", nil),
		width:    100,
		height:   40,
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch selection state")
	}
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(tuiModel)
	if updated.state != tuiStatePromptConfig || updated.pendingBranch == "" {
		t.Fatalf("expected prompt config state")
	}
}

func TestTUIDeleteDirty(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "status" {
			return cmdWithOutput(" M file.txt")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated := next.(tuiModel)
	if updated.status != "worktree has uncommitted changes" {
		t.Fatalf("unexpected status: %q", updated.status)
	}
}

func TestTUIDeleteClean(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "status" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateConfirmDelete || updated.pendingDelete.path != "/repo" {
		t.Fatalf("expected confirm delete state")
	}
}

func TestTUIDeleteError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated := next.(tuiModel)
	if updated.status == "" {
		t.Fatalf("expected status error")
	}
}

func TestTUIPromptConfigLibs(t *testing.T) {
	model := tuiModel{state: tuiStatePromptConfig}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if !updated.copyConfig || updated.state != tuiStatePromptLibs {
		t.Fatalf("expected prompt libs with copyConfig true")
	}

	model = tuiModel{state: tuiStatePromptConfig}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated = next.(tuiModel)
	if updated.copyConfig || updated.state != tuiStatePromptLibs {
		t.Fatalf("expected prompt libs with copyConfig false")
	}

	model = tuiModel{state: tuiStatePromptConfig}
	next, _ = model.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	updated = next.(tuiModel)
	if updated.state != tuiStatePromptConfig {
		t.Fatalf("expected state unchanged")
	}
}

func TestTUIFinishCreate(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:         tuiStatePromptLibs,
		repoRoot:      repo,
		pendingBranch: "main",
		copyConfig:    true,
		copyLibs:      false,
		list:          newListModel("Worktrees", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}

	next, _ = updated.Update(createResultMsg{err: nil})
	updated = next.(tuiModel)
	if updated.state != tuiStateList || updated.status == "" {
		t.Fatalf("expected list state with status, got %v %q", updated.state, updated.status)
	}
}

func TestTUIFinishCreateCopyLibs(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:         tuiStatePromptLibs,
		repoRoot:      repo,
		pendingBranch: "main",
		copyConfig:    false,
		copyLibs:      true,
		list:          newListModel("Worktrees", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}

	next, _ = updated.Update(createResultMsg{err: nil})
	updated = next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state")
	}
}

func TestReloadWorktreesError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	model := tuiModel{
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	if err := model.reloadWorktrees(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestReloadWorktreesSuccess(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	out := strings.Join([]string{
		"worktree /repo",
		"branch refs/heads/main",
		"",
	}, "\n")
	execCommand = func(name string, args ...string) *exec.Cmd {
		return cmdWithOutput(out)
	}
	model := tuiModel{
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	if err := model.reloadWorktrees(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(model.list.Items()) != 1 {
		t.Fatalf("expected items to load")
	}
}

func TestCreateWorktreeNewBranch(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	addedWithB := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			for _, arg := range args {
				if arg == "-b" {
					addedWithB = true
				}
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "feature",
		copyConfig:    false,
		copyLibs:      false,
	}
	if err := model.createWorktree(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !addedWithB {
		t.Fatalf("expected worktree add -b to be used")
	}
}

func TestCreateWorktreeMkdirError(t *testing.T) {
	repo := t.TempDir()

	oldMkdir := osMkdirAll
	defer func() { osMkdirAll = oldMkdir }()

	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("mkdir fail")
	}

	model := tuiModel{
		repoRoot:      repo,
		pendingBranch: "main",
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateWorktreeBranchExistsError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("does-not-exist")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateWorktreeAddErrorExists(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateWorktreeAddErrorNew(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateWorktreeCopyConfigError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldStat := osStat
	defer func() {
		execCommand = oldExec
		osStat = oldStat
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osStat = func(name string) (fs.FileInfo, error) {
		return nil, errors.New("stat fail")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
		copyConfig:    true,
		copyLibs:      false,
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateWorktreeCopyMatchingFilesError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldWalk := filepathWalkDir
	defer func() {
		execCommand = oldExec
		filepathWalkDir = oldWalk
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return errors.New("walk fail")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
		copyConfig:    true,
		copyLibs:      false,
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCreateWorktreeCopyLibsError(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	oldStat := osStat
	defer func() {
		execCommand = oldExec
		osStat = oldStat
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 0")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osStat = func(name string) (fs.FileInfo, error) {
		return nil, errors.New("stat fail")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
		copyConfig:    false,
		copyLibs:      true,
	}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestTUIFinishCreateError(t *testing.T) {
	model := tuiModel{
		state:         tuiStatePromptLibs,
		repoRoot:      "/repo",
		pendingBranch: "",
		copyConfig:    true,
		list:          newListModel("Worktrees", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}

	next, _ = updated.Update(createResultMsg{err: errors.New("boom")})
	updated = next.(tuiModel)
	if updated.state != tuiStateList || updated.status == "" {
		t.Fatalf("expected error status")
	}
}

func TestTUIBranchErrors(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.status == "" {
		t.Fatalf("expected status error")
	}
}

func TestTUIBranchNoBranches(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "branch" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.status != "no branches found" {
		t.Fatalf("unexpected status: %q", updated.status)
	}
}

func TestTUIBranchEsc(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state")
	}
}

func TestTUIBranchEnterNoSelection(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch state")
	}
}

func TestTUIBranchNavigation(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", []list.Item{branchItem("main"), branchItem("dev")}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated := next.(tuiModel)
	if updated.branches.Index() != 1 {
		t.Fatalf("expected index 1, got %d", updated.branches.Index())
	}
}

func TestTUIBranchListUpdateFilterInput(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
	}
	// Enter filter mode first
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(tuiModel)
	// Now type into filter
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated = next.(tuiModel)
	if updated.branches.FilterValue() != "x" {
		t.Fatalf("expected filter value 'x', got %q", updated.branches.FilterValue())
	}
}

func TestTUIBranchCreateFlow(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "branch" {
			return cmdWithOutput("main\nfeature")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:    tuiStateList,
		repoRoot: repo,
		list:     newListModel("Worktrees", nil),
		width:    100,
		height:   40,
	}
	// Go to branch list
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch selection state")
	}
	// Press 'c' to create from selected branch
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	updated = next.(tuiModel)
	if updated.state != tuiStateInputBranchName {
		t.Fatalf("expected input branch name state, got %v", updated.state)
	}
	if updated.baseBranch == "" {
		t.Fatalf("expected baseBranch to be set")
	}
	// Type branch name
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	updated = next.(tuiModel)
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated = next.(tuiModel)
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated = next.(tuiModel)
	// Confirm
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(tuiModel)
	if updated.state != tuiStateConfirmNewBranch {
		t.Fatalf("expected confirm new branch state, got %v", updated.state)
	}
	if updated.pendingBranch != "foo" {
		t.Fatalf("expected pendingBranch 'foo', got %q", updated.pendingBranch)
	}
	// Accept
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(tuiModel)
	if updated.state != tuiStatePromptConfig {
		t.Fatalf("expected prompt config state, got %v", updated.state)
	}
}

func TestTUIBranchCreateEsc(t *testing.T) {
	model := tuiModel{
		state:      tuiStateInputBranchName,
		baseBranch: "main",
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch state, got %v", updated.state)
	}
	if updated.baseBranch != "" {
		t.Fatalf("expected baseBranch cleared")
	}
}

func TestTUIBranchCreateEmptyEnter(t *testing.T) {
	model := tuiModel{
		state:      tuiStateInputBranchName,
		baseBranch: "main",
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.state != tuiStateInputBranchName {
		t.Fatalf("expected state unchanged for empty input, got %v", updated.state)
	}
}

func TestTUIBranchCreateInputNonKey(t *testing.T) {
	model := tuiModel{
		state:      tuiStateInputBranchName,
		baseBranch: "main",
	}
	next, _ := model.Update(spinner.TickMsg{})
	updated := next.(tuiModel)
	if updated.state != tuiStateInputBranchName {
		t.Fatalf("expected state unchanged")
	}
}

func TestTUIConfirmNewBranchCancel(t *testing.T) {
	model := tuiModel{
		state:         tuiStateConfirmNewBranch,
		baseBranch:    "main",
		pendingBranch: "feature",
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch state, got %v", updated.state)
	}
	if updated.baseBranch != "" || updated.pendingBranch != "" {
		t.Fatalf("expected baseBranch/pendingBranch cleared")
	}
}

func TestTUIConfirmNewBranchNonKey(t *testing.T) {
	model := tuiModel{
		state:         tuiStateConfirmNewBranch,
		baseBranch:    "main",
		pendingBranch: "feature",
	}
	next, _ := model.Update(spinner.TickMsg{})
	updated := next.(tuiModel)
	if updated.state != tuiStateConfirmNewBranch {
		t.Fatalf("expected state unchanged")
	}
}

func TestTUIBranchCreateCNoSelection(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected state unchanged, got %v", updated.state)
	}
}

func TestTUIBranchEnterClearsBaseBranch(t *testing.T) {
	model := tuiModel{
		state:      tuiStateNewBranch,
		repoRoot:   "/repo",
		branches:   newListModel("Select branch", []list.Item{branchItem("main")}),
		baseBranch: "develop",
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(tuiModel)
	if updated.baseBranch != "" {
		t.Fatalf("expected baseBranch cleared, got %q", updated.baseBranch)
	}
}

func TestCreateWorktreeWithBaseBranch(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	var gotArgs []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			gotArgs = args
			return exec.Command("sh", "-c", "exit 0")
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
	if err := model.createWorktree(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have: worktree add -b feature <path> develop
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
		t.Fatalf("expected -b and baseBranch in args, got %v", gotArgs)
	}
}

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

func TestTUIPromptEsc(t *testing.T) {
	model := tuiModel{state: tuiStatePromptConfig}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state")
	}
}

func TestTUIPromptLibsEsc(t *testing.T) {
	model := tuiModel{state: tuiStatePromptLibs}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state")
	}
}

func TestTUIPromptLibsNonKey(t *testing.T) {
	model := tuiModel{state: tuiStatePromptLibs}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	updated := next.(tuiModel)
	if updated.state != tuiStatePromptLibs {
		t.Fatalf("expected state unchanged")
	}
}

func TestTUIConfirmDeleteCancel(t *testing.T) {
	model := tuiModel{
		state:         tuiStateConfirmDelete,
		pendingDelete: worktreeItem{branch: "main", path: "/repo"},
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateList || updated.pendingDelete.path != "" {
		t.Fatalf("expected cancel delete")
	}
}

func TestTUIConfirmDeleteSuccess(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput("")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		state:         tuiStateConfirmDelete,
		repoRoot:      "/repo",
		pendingDelete: worktreeItem{branch: "main", path: "/repo"},
		list:          newListModel("Worktrees", nil),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}

	next, _ = updated.Update(deleteResultMsg{err: nil})
	updated = next.(tuiModel)
	if updated.state != tuiStateList || updated.status == "" {
		t.Fatalf("expected delete success")
	}
}

func TestTUIConfirmDeleteNonKey(t *testing.T) {
	model := tuiModel{state: tuiStateConfirmDelete}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	updated := next.(tuiModel)
	if updated.state != tuiStateConfirmDelete {
		t.Fatalf("expected state unchanged")
	}
}

func TestTUIBusyIgnoresKeys(t *testing.T) {
	model := tuiModel{state: tuiStateBusy}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}
}

func TestCreateWorktreeCmd(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      repo,
		mainWorktree:  repo,
		pendingBranch: "main",
	}
	msg := createWorktreeCmd(model)()
	if _, ok := msg.(createResultMsg); !ok {
		t.Fatalf("expected createResultMsg")
	}
}

func TestDeleteWorktreeCmd(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 0")
	}

	model := tuiModel{
		repoRoot:      "/repo",
		pendingDelete: worktreeItem{path: "/repo"},
	}
	msg := deleteWorktreeCmd(model)()
	if _, ok := msg.(deleteResultMsg); !ok {
		t.Fatalf("expected deleteResultMsg")
	}
}

func TestCreateWorktreeEmptyBranch(t *testing.T) {
	model := tuiModel{repoRoot: "/repo"}
	if err := model.createWorktree(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestTUISpinnerTick(t *testing.T) {
	model := tuiModel{state: tuiStateBusy, spinner: spinner.New()}
	next, _ := model.Update(spinner.TickMsg{})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}
}

func TestTUIBusyWindowSize(t *testing.T) {
	model := tuiModel{state: tuiStateBusy}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}
}

func TestTUIUpdateQuit(t *testing.T) {
	model := tuiModel{state: tuiStateList}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := next.(tuiModel)
	if updated.action.kind != tuiActionNone {
		t.Fatalf("expected quit action")
	}
}

func TestTUIUpdateView(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
		status:   "ok",
	}
	if model.View() == "" {
		t.Fatalf("expected list view")
	}
	model.state = tuiStateNewBranch
	if model.View() == "" {
		t.Fatalf("expected branch view")
	}
	model.state = tuiStatePromptConfig
	if model.View() == "" {
		t.Fatalf("expected prompt view")
	}
	model.state = tuiStatePromptLibs
	if model.View() == "" {
		t.Fatalf("expected prompt view")
	}
	model.state = tuiStateConfirmDelete
	if model.View() == "" {
		t.Fatalf("expected confirm view")
	}
	model.state = tuiStateInputBranchName
	model.baseBranch = "main"
	if model.View() == "" {
		t.Fatalf("expected input branch name view")
	}
	model.state = tuiStateConfirmNewBranch
	model.pendingBranch = "feature"
	if model.View() == "" {
		t.Fatalf("expected confirm new branch view")
	}
	model.state = tuiStateBusy
	if model.View() == "" {
		t.Fatalf("expected busy view")
	}
	model.state = tuiState(99)
	if model.View() != "" {
		t.Fatalf("expected empty view")
	}
}

func TestTUIUpdateWindowSize(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
	}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated := next.(tuiModel)
	if updated.list.Width() == 0 {
		t.Fatalf("expected list width")
	}
	updated.state = tuiStateNewBranch
	next, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated = next.(tuiModel)
	if updated.branches.Width() == 0 {
		t.Fatalf("expected branches width")
	}
}

func TestTUIUpdateDefaultState(t *testing.T) {
	model := tuiModel{state: tuiState(99)}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated := next.(tuiModel)
	if updated.state != tuiState(99) {
		t.Fatalf("expected state unchanged")
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

func TestCopyItemsAndCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.MkdirAll(filepath.Join(src, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "node_modules", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("env"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := copyItems(src, dst, []string{"node_modules", ".env", "missing"}); err != nil {
		t.Fatalf("copy items: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules", "a.txt")); err != nil {
		t.Fatalf("expected copied dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".env")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}

func TestCopyItemsStatError(t *testing.T) {
	oldStat := osStat
	defer func() { osStat = oldStat }()
	osStat = func(name string) (fs.FileInfo, error) {
		return nil, errors.New("stat fail")
	}

	if err := copyItems("/src", "/dst", []string{"file"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCopyItemsCopyDirError(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	oldWalk := filepathWalkDir
	defer func() { filepathWalkDir = oldWalk }()
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return errors.New("walk fail")
	}

	if err := copyItems(src, t.TempDir(), []string{"node_modules"}); err == nil {
		t.Fatalf("expected copy dir error")
	}
}

func TestCopyItemsCopyFileError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	filePath := filepath.Join(src, ".env")

	if err := os.WriteFile(filePath, []byte("env"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldOpen := osOpen
	defer func() { osOpen = oldOpen }()
	osOpen = func(name string) (*os.File, error) {
		return nil, errors.New("open fail")
	}

	if err := copyItems(src, dst, []string{".env"}); err == nil {
		t.Fatalf("expected copy file error")
	}
}

func TestCopyDirErrors(t *testing.T) {
	oldWalk := filepathWalkDir
	oldMkdir := osMkdirAll
	oldStderr := stderr
	defer func() {
		filepathWalkDir = oldWalk
		osMkdirAll = oldMkdir
		stderr = oldStderr
	}()

	// Walk error should warn and continue
	var buf bytes.Buffer
	stderr = &buf
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk fail"))
	}
	if err := copyDir("/src", "/dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "warning:") {
		t.Fatalf("expected warning, got %q", buf.String())
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "file"), fakeDirEntry{name: "file", isDir: false, infoErr: errors.New("info fail")}, nil)
	}
	if err := copyDir("root", "/dst"); err == nil {
		t.Fatalf("expected info error")
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn("dir", fakeDirEntry{name: "dir", isDir: true}, nil)
	}
	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("mkdir fail")
	}
	if err := copyDir("/src", "/dst"); err == nil {
		t.Fatalf("expected mkdir error")
	}

	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn("file", fakeDirEntry{name: "file", isDir: false}, nil)
	}
	if err := copyDir("", "/dst"); err == nil {
		t.Fatalf("expected rel error")
	}
}

func TestCopyMatchingFilesSuccess(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create .env in root and subdirectory
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("ROOT"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", ".env"), []byte("SUB"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Create a file that shouldn't be copied
	if err := os.WriteFile(filepath.Join(src, "other.txt"), []byte("OTHER"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := copyMatchingFiles(src, dst, []string{".env"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check root .env
	content, err := os.ReadFile(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatalf("read root .env: %v", err)
	}
	if string(content) != "ROOT" {
		t.Fatalf("expected ROOT, got %q", content)
	}

	// Check sub/.env
	content, err = os.ReadFile(filepath.Join(dst, "sub", ".env"))
	if err != nil {
		t.Fatalf("read sub/.env: %v", err)
	}
	if string(content) != "SUB" {
		t.Fatalf("expected SUB, got %q", content)
	}

	// Check other.txt was not copied
	if _, err := os.Stat(filepath.Join(dst, "other.txt")); !os.IsNotExist(err) {
		t.Fatalf("other.txt should not be copied")
	}
}

func TestCopyMatchingFilesErrors(t *testing.T) {
	oldWalk := filepathWalkDir
	oldStderr := stderr
	defer func() {
		filepathWalkDir = oldWalk
		stderr = oldStderr
	}()

	// Walk error should warn and continue
	var buf bytes.Buffer
	stderr = &buf
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk fail"))
	}
	if err := copyMatchingFiles("/src", "/dst", []string{".env"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "warning:") {
		t.Fatalf("expected warning, got %q", buf.String())
	}

	// Info error
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, ".env"), fakeDirEntry{name: ".env", isDir: false, infoErr: errors.New("info fail")}, nil)
	}
	if err := copyMatchingFiles("/src", "/dst", []string{".env"}); err == nil {
		t.Fatalf("expected info error")
	}

	// Rel error (relative root with absolute path)
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn("/absolute/path/.env", fakeDirEntry{name: ".env", isDir: false}, nil)
	}
	if err := copyMatchingFiles("relative", "/dst", []string{".env"}); err == nil {
		t.Fatalf("expected rel error")
	}
}

func TestCopyMatchingFilesCopyError(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldOpen := osOpen
	defer func() { osOpen = oldOpen }()
	osOpen = func(name string) (*os.File, error) {
		return nil, errors.New("open fail")
	}

	if err := copyMatchingFiles(src, t.TempDir(), []string{".env"}); err == nil {
		t.Fatalf("expected copy error")
	}
}

func TestCopyFileErrors(t *testing.T) {
	oldMkdir := osMkdirAll
	oldOpen := osOpen
	oldOpenFile := osOpenFile
	oldCopy := ioCopy
	defer func() {
		osMkdirAll = oldMkdir
		osOpen = oldOpen
		osOpenFile = oldOpenFile
		ioCopy = oldCopy
	}()

	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("mkdir fail")
	}
	if err := copyFile("src", "dst", 0o644); err == nil {
		t.Fatalf("expected mkdir error")
	}

	osMkdirAll = oldMkdir
	osOpen = func(name string) (*os.File, error) {
		return nil, errors.New("open fail")
	}
	if err := copyFile("src", "dst", 0o644); err == nil {
		t.Fatalf("expected open error")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	osOpen = func(name string) (*os.File, error) {
		return os.Open(src)
	}
	osOpenFile = func(name string, flag int, perm fs.FileMode) (*os.File, error) {
		return nil, errors.New("openfile fail")
	}
	if err := copyFile(src, filepath.Join(tmp, "dst.txt"), 0o644); err == nil {
		t.Fatalf("expected openfile error")
	}

	osOpenFile = oldOpenFile
	ioCopy = func(dst io.Writer, src io.Reader) (int64, error) {
		return 0, errors.New("copy fail")
	}
	if err := copyFile(src, filepath.Join(tmp, "dst2.txt"), 0o644); err == nil {
		t.Fatalf("expected copy error")
	}
}

func TestCopyFileSuccess(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")

	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatalf("copy: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("unexpected data %q", string(data))
	}
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
	cmd := exec.Command("git", "commit", "-m", "newer commit")
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

func TestSlugify(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"Fix login timeout", 50, "fix-login-timeout"},
		{"Hello, World! 123", 50, "hello-world-123"},
		{"  --leading trailing--  ", 50, "leading-trailing"},
		{"A very long title that should be truncated at word boundary here", 30, "a-very-long-title-that-should"},
		{"", 50, ""},
		{"---", 50, ""},
	}
	for _, tt := range tests {
		got := slugify(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("slugify(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestJiraBranchName(t *testing.T) {
	tests := []struct {
		key     string
		summary string
		want    string
	}{
		{"PROJ-123", "Fix login timeout", "PROJ-123-fix-login-timeout"},
		{"PROJ-456", "", "PROJ-456"},
		{"PROJ-789", "---", "PROJ-789"},
	}
	for _, tt := range tests {
		got := jiraBranchName(tt.key, tt.summary)
		if got != tt.want {
			t.Errorf("jiraBranchName(%q, %q) = %q, want %q", tt.key, tt.summary, got, tt.want)
		}
	}
}

func TestRenderIssueMD(t *testing.T) {
	// Full issue
	issue := jiraIssue{
		Key: "PROJ-123",
		Fields: jiraFields{
			Summary:     "Fix login timeout",
			Description: "Users see a timeout after 30s.",
			Comment: jiraComments{
				Comments: []jiraComment{
					{
						Author:  jiraAuthor{DisplayName: "Jane Smith"},
						Body:    "I can reproduce this.",
						Created: "2024-01-15T10:30:00.000+0000",
					},
				},
			},
		},
	}
	md := renderIssueMD(issue)
	if !strings.Contains(md, "# PROJ-123: Fix login timeout") {
		t.Fatalf("expected title in md: %s", md)
	}
	if !strings.Contains(md, "## Description") {
		t.Fatalf("expected description section: %s", md)
	}
	if !strings.Contains(md, "## Comments") {
		t.Fatalf("expected comments section: %s", md)
	}
	if !strings.Contains(md, "Jane Smith") {
		t.Fatalf("expected author in md: %s", md)
	}

	// No description, no comments
	issue2 := jiraIssue{
		Key:    "PROJ-456",
		Fields: jiraFields{Summary: "Simple bug"},
	}
	md2 := renderIssueMD(issue2)
	if strings.Contains(md2, "## Description") {
		t.Fatalf("expected no description section: %s", md2)
	}
	if strings.Contains(md2, "## Comments") {
		t.Fatalf("expected no comments section: %s", md2)
	}

	// Description only
	issue3 := jiraIssue{
		Key:    "PROJ-789",
		Fields: jiraFields{Summary: "With desc", Description: "Some desc"},
	}
	md3 := renderIssueMD(issue3)
	if !strings.Contains(md3, "## Description") {
		t.Fatalf("expected description: %s", md3)
	}
	if strings.Contains(md3, "## Comments") {
		t.Fatalf("expected no comments: %s", md3)
	}

	// Comments only
	issue4 := jiraIssue{
		Key: "PROJ-000",
		Fields: jiraFields{
			Summary: "With comments",
			Comment: jiraComments{
				Comments: []jiraComment{
					{Author: jiraAuthor{DisplayName: "Bob"}, Body: "test", Created: "2024-01-01T00:00:00.000+0000"},
				},
			},
		},
	}
	md4 := renderIssueMD(issue4)
	if strings.Contains(md4, "## Description") {
		t.Fatalf("expected no description: %s", md4)
	}
	if !strings.Contains(md4, "## Comments") {
		t.Fatalf("expected comments: %s", md4)
	}
}

func TestJiraGetDefaultSuccess(t *testing.T) {
	issue := jiraIssue{Key: "TEST-1", Fields: jiraFields{Summary: "Test"}}
	body, _ := json.Marshal(issue)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	got, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/TEST-1", "user", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("unexpected body: %s", string(got))
	}
}

func TestJiraGetDefault404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/NOPE-1", "user", "token")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestJiraGetDefault401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/TEST-1", "bad", "creds")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestJiraGetDefaultUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/TEST-1", "user", "token")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

func TestJiraGetDefaultNetworkError(t *testing.T) {
	_, err := jiraGetDefault("http://127.0.0.1:1/bad", "user", "token")
	if err == nil {
		t.Fatalf("expected network error")
	}
}

func TestJiraCmdSuccess(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	var writtenPath string
	var writtenData []byte
	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		writtenPath = name
		writtenData = data
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"PROJ-123"})

	wtPath := worktreePath(repo, "PROJ-123-fix-login")
	if !strings.Contains(buf.String(), wtPath) {
		t.Fatalf("expected wtPath in output, got %q", buf.String())
	}
	if writtenPath != filepath.Join(wtPath, "PROJ-123.md") {
		t.Fatalf("expected md at %s, got %s", filepath.Join(wtPath, "PROJ-123.md"), writtenPath)
	}
	if !strings.Contains(string(writtenData), "# PROJ-123: Fix login") {
		t.Fatalf("expected issue content in md, got %s", string(writtenData))
	}
}

func TestJiraCmdBranchOverride(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"-b", "my-branch", "PROJ-123"})

	wtPath := worktreePath(repo, "my-branch")
	if !strings.Contains(buf.String(), wtPath) {
		t.Fatalf("expected wtPath with custom branch in output, got %q", buf.String())
	}
}

func TestJiraCmdTmux(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldTmuxEnv := os.Getenv("TMUX")
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
		_ = os.Setenv("TMUX", oldTmuxEnv)
	}()

	_ = os.Unsetenv("TMUX")

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	tmuxCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			tmuxCalled = true
			return exec.Command("sh", "-c", "exit 0")
		}
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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"-t", "PROJ-123"})

	if !tmuxCalled {
		t.Fatalf("expected tmux to be called")
	}
}

func TestJiraCmdMissingIssueKey(t *testing.T) {
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
		if !strings.Contains(buf.String(), "issue key required") {
			t.Fatalf("expected issue key error, got %q", buf.String())
		}
	}()

	jiraCmd(nil)
}

func TestJiraCmdMissingEnvVars(t *testing.T) {
	oldGetenv := osGetenv
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string { return "" }

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "JIRA_URL") {
			t.Fatalf("expected env var error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"PROJ-123"})
}

func TestJiraCmdAPIError(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	jiraGet = func(url, user, token string) ([]byte, error) {
		return nil, errors.New("jira: issue not found (404)")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "404") {
			t.Fatalf("expected 404 error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"PROJ-123"})
}

func TestJiraCmdInvalidJSON(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	jiraGet = func(url, user, token string) ([]byte, error) {
		return []byte("not json"), nil
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "invalid response") {
			t.Fatalf("expected invalid response error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"PROJ-123"})
}

func TestJiraCmdWriteError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return errors.New("write fail")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "write fail") {
			t.Fatalf("expected write error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"PROJ-123"})
}

func TestJiraCmdAddWorktreeError(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraCmd([]string{"PROJ-123"})
}

func TestJiraCmdTmuxError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldExit := exitFunc
	oldErr := stderr
	oldTmuxEnv := os.Getenv("TMUX")
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		exitFunc = oldExit
		stderr = oldErr
		_ = os.Setenv("TMUX", oldTmuxEnv)
	}()

	_ = os.Unsetenv("TMUX")

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			return exec.Command("sh", "-c", "exit 1")
		}
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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraCmd([]string{"-t", "PROJ-123"})
}

func TestJiraCmdTrailingSlashURL(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com/"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	var gotURL string
	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Test"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		gotURL = url
		return body, nil
	}

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"PROJ-123"})

	if strings.Contains(gotURL, "//rest") {
		t.Fatalf("expected trailing slash stripped, got URL %q", gotURL)
	}
}

func TestMainHelpIncludesJira(t *testing.T) {
	oldErr := stderr
	defer func() { stderr = oldErr }()

	var buf bytes.Buffer
	stderr = &buf
	printUsage()

	if !strings.Contains(buf.String(), "jira") {
		t.Fatalf("expected jira in usage output, got %q", buf.String())
	}
}

func TestAddWorktreeSuccess(t *testing.T) {
	repo := t.TempDir()

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	wtPath, err := addWorktree("test-branch", "", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := worktreePath(repo, "test-branch")
	if wtPath != expected {
		t.Fatalf("expected %q, got %q", expected, wtPath)
	}
}

func TestAddWorktreeRepoRootError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	_, err := addWorktree("test-branch", "", true, false)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestJiraCmdNoCopyConfig(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"-C", "PROJ-123"})

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestJiraGetDefaultInvalidURL(t *testing.T) {
	_, err := jiraGetDefault("://bad\x7f", "user", "token")
	if err == nil {
		t.Fatalf("expected error for invalid URL")
	}
}

func TestJiraCmdNoCopyLibs(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

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
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"-L", "PROJ-123"})

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}
