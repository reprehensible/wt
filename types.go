package main

// worktree represents a git worktree with its path and branch.
type worktree struct {
	Path   string
	Branch string
}

type tuiState int

const (
	tuiStateList tuiState = iota
	tuiStateNewBranch
	tuiStatePromptConfig
	tuiStatePromptLibs
	tuiStateConfirmDelete
	tuiStateInputBranchName
	tuiStateConfirmNewBranch
	tuiStateBusy
	tuiStateHelp
)

const (
	tuiActionNone = ""
	tuiActionGo   = "go"
	tuiActionTmux = "tmux"
)

type tuiAction struct {
	kind string
	path string
}

type worktreeItem struct {
	branch  string
	path    string
	display string
}

func (w worktreeItem) Title() string {
	if w.display != "" {
		return w.display
	}
	return w.path
}

func (w worktreeItem) Description() string { return "" }
func (w worktreeItem) FilterValue() string {
	if w.branch != "" {
		return w.branch + " " + w.path
	}
	return w.path
}

type branchItem string

func (b branchItem) Title() string       { return string(b) }
func (b branchItem) Description() string { return "" }
func (b branchItem) FilterValue() string { return string(b) }
