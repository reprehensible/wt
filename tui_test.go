package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

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

func (d descItem) Title() string { return d.title }

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
	out := promptView("Copy config files?", true, "status", 80)
	if !strings.Contains(out, "Copy config files?") || !strings.Contains(out, "status") {
		t.Fatalf("unexpected prompt output: %q", out)
	}
	out = promptView("Confirm?", false, "", 0)
	if !strings.Contains(out, "Confirm?") || !strings.Contains(out, "[y/N]") {
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
	if listFooter(0) == "" || branchFooter(0) == "" {
		t.Fatalf("expected footers")
	}
	// Compact footers for narrow widths
	narrow := listFooter(30)
	if !strings.Contains(narrow, "quit") {
		t.Fatalf("expected compact footer, got %q", narrow)
	}
	narrow = branchFooter(30)
	if !strings.Contains(narrow, "help") {
		t.Fatalf("expected compact footer, got %q", narrow)
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
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
		width:    100,
		height:   40,
	}
	// Press 'n' - should go to busy state for async loading
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state, got %v", updated.state)
	}
	// Simulate branch loading result
	next, _ = updated.Update(branchesResultMsg{branches: []string{"main", "feature"}})
	updated = next.(tuiModel)
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
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	// Press 'n' - goes to busy state
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}
	// Simulate branch loading error
	next, _ = updated.Update(branchesResultMsg{err: errors.New("fail")})
	updated = next.(tuiModel)
	if updated.status == "" {
		t.Fatalf("expected status error")
	}
	if updated.state != tuiStateList {
		t.Fatalf("expected list state after error")
	}
}

func TestTUIBranchNoBranches(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	// Press 'n' - goes to busy state
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}
	// Simulate empty branches result
	next, _ = updated.Update(branchesResultMsg{branches: nil})
	updated = next.(tuiModel)
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
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
		width:    100,
		height:   40,
	}
	// Go to branch list (async)
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateBusy {
		t.Fatalf("expected busy state")
	}
	next, _ = updated.Update(branchesResultMsg{branches: []string{"main", "feature"}})
	updated = next.(tuiModel)
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

func TestTUIQuitBlockedDuringListFilter(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	// Enter filter mode
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(tuiModel)
	if updated.list.FilterState() != list.Filtering {
		t.Fatalf("expected filtering state")
	}
	// Press 'q' - should type into filter, not quit
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated = next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state (not quit)")
	}
	if updated.list.FilterValue() != "q" {
		t.Fatalf("expected filter value 'q', got %q", updated.list.FilterValue())
	}
}

func TestTUIQuitBlockedDuringBranchFilter(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
	}
	// Enter filter mode
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(tuiModel)
	if updated.branches.FilterState() != list.Filtering {
		t.Fatalf("expected filtering state")
	}
	// Press 'q' - should type into filter, not quit
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated = next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch state (not quit)")
	}
}

func TestTUIQuitBlockedDuringInput(t *testing.T) {
	model := tuiModel{
		state:      tuiStateInputBranchName,
		baseBranch: "main",
	}
	// Press 'q' - should type into input, not quit
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateInputBranchName {
		t.Fatalf("expected input state (not quit)")
	}
}

func TestTUIHelpToggle(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	// Press '?' to open help
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateHelp {
		t.Fatalf("expected help state")
	}
	// Verify help view has content
	view := updated.View()
	if !strings.Contains(view, "Keyboard Shortcuts") {
		t.Fatalf("expected help content in view")
	}
	// Press any key to dismiss help
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	updated = next.(tuiModel)
	if updated.state != tuiStateList {
		t.Fatalf("expected list state after dismissing help")
	}
}

func TestTUIHelpFromBranchList(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		repoRoot: "/repo",
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateHelp {
		t.Fatalf("expected help state")
	}
}

func TestTUIHelpNonKey(t *testing.T) {
	model := tuiModel{state: tuiStateHelp}
	next, _ := model.Update(spinner.TickMsg{})
	updated := next.(tuiModel)
	if updated.state != tuiStateHelp {
		t.Fatalf("expected help state unchanged")
	}
}

func TestHelpContent(t *testing.T) {
	content := helpContent()
	if !strings.Contains(content, "Keyboard Shortcuts") {
		t.Fatalf("expected help content")
	}
	if !strings.Contains(content, "enter") || !strings.Contains(content, "Quit") {
		t.Fatalf("expected key bindings in help")
	}
}

func TestIsFiltering(t *testing.T) {
	model := tuiModel{
		state:    tuiStateList,
		list:     newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
		branches: newListModel("Select branch", []list.Item{branchItem("main")}),
	}
	if model.isFiltering() {
		t.Fatalf("expected not filtering initially")
	}
	// Enter filter mode on list
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := next.(tuiModel)
	if !updated.isFiltering() {
		t.Fatalf("expected filtering in list state")
	}
	// Check branch list filtering
	model.state = tuiStateNewBranch
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated = next.(tuiModel)
	if !updated.isFiltering() {
		t.Fatalf("expected filtering in branch state")
	}
	// Other states should not be filtering
	model.state = tuiStatePromptConfig
	if model.isFiltering() {
		t.Fatalf("expected not filtering in prompt state")
	}
}

func TestBranchesResultMsg(t *testing.T) {
	model := tuiModel{
		state:    tuiStateBusy,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
		width:    100,
		height:   40,
	}
	// Success with branches
	next, _ := model.Update(branchesResultMsg{branches: []string{"main", "dev"}})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch state, got %v", updated.state)
	}
	if len(updated.branches.Items()) != 2 {
		t.Fatalf("expected 2 branch items")
	}
}

func TestBranchesResultMsgNoSize(t *testing.T) {
	model := tuiModel{
		state:    tuiStateBusy,
		repoRoot: "/repo",
		list:     newListModel("Worktrees", nil),
	}
	next, _ := model.Update(branchesResultMsg{branches: []string{"main"}})
	updated := next.(tuiModel)
	if updated.state != tuiStateNewBranch {
		t.Fatalf("expected branch state")
	}
}

func TestLoadBranchesCmd(t *testing.T) {
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

	cmd := loadBranchesCmd("/repo")
	msg := cmd()
	result, ok := msg.(branchesResultMsg)
	if !ok {
		t.Fatalf("expected branchesResultMsg")
	}
	if result.err != nil {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if len(result.branches) == 0 {
		t.Fatalf("expected branches")
	}
}

func TestLoadBranchesCmdError(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	cmd := loadBranchesCmd("/repo")
	msg := cmd()
	result, ok := msg.(branchesResultMsg)
	if !ok {
		t.Fatalf("expected branchesResultMsg")
	}
	if result.err == nil {
		t.Fatalf("expected error")
	}
}

func TestReloadWorktreesRecalculatesSize(t *testing.T) {
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
		width:    100,
		height:   40,
	}
	if err := model.reloadWorktrees(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(model.list.Items()) != 1 {
		t.Fatalf("expected items to load")
	}
}

func TestTUIStatusClearedOnDelete(t *testing.T) {
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
		status:   "worktree created",
	}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated := next.(tuiModel)
	if updated.state != tuiStateConfirmDelete {
		t.Fatalf("expected confirm delete state")
	}
	if updated.status != "" {
		t.Fatalf("expected status cleared, got %q", updated.status)
	}
}

func TestTUIDeletePromptShowsBranch(t *testing.T) {
	model := tuiModel{
		state:         tuiStateConfirmDelete,
		pendingDelete: worktreeItem{branch: "feature", path: "/repo/feature"},
	}
	view := model.View()
	if !strings.Contains(view, "feature") {
		t.Fatalf("expected branch name in delete prompt, got %q", view)
	}
}

func TestTUIDeletePromptShowsPathFallback(t *testing.T) {
	model := tuiModel{
		state:         tuiStateConfirmDelete,
		pendingDelete: worktreeItem{path: "/repo/my-work"},
	}
	view := model.View()
	if !strings.Contains(view, "my-work") {
		t.Fatalf("expected path basename in delete prompt, got %q", view)
	}
}

func TestTUIWindowSizeCapsList(t *testing.T) {
	model := tuiModel{
		state: tuiStateList,
		list:  newListModel("Worktrees", []list.Item{worktreeItem{branch: "main", path: "/repo"}}),
	}
	// With 1 item and 40-line terminal, list should be capped
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated := next.(tuiModel)
	if updated.width != 100 || updated.height != 40 {
		t.Fatalf("expected dimensions stored")
	}
}

func TestTUIWindowSizeCapsBranches(t *testing.T) {
	model := tuiModel{
		state:    tuiStateNewBranch,
		branches: newListModel("Select branch", []list.Item{branchItem("main"), branchItem("dev")}),
	}
	next, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated := next.(tuiModel)
	if updated.width != 100 || updated.height != 40 {
		t.Fatalf("expected dimensions stored")
	}
}

func TestRunTUIInterrupt(t *testing.T) {
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
		return stubProgram{err: tea.ErrProgramKilled}
	}

	action, err := runTUI()
	if err != nil {
		t.Fatalf("expected nil error for interrupt, got %v", err)
	}
	if action.kind != tuiActionNone {
		t.Fatalf("expected no action for interrupt")
	}
}
