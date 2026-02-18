package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type programRunner interface {
	Run() (tea.Model, error)
}

var (
	frameStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	titleStyle  = lipgloss.NewStyle().Bold(true).PaddingLeft(1)
	headerStyle = lipgloss.NewStyle().Faint(true)
)

type tuiModel struct {
	state         tuiState
	repoRoot      string
	mainWorktree  string
	list          list.Model
	branches      list.Model
	status        string
	pendingBranch string
	pendingDelete worktreeItem
	copyConfig    bool
	copyLibs      bool
	baseBranch    string
	input         textinput.Model
	busyText      string
	spinner       spinner.Model
	action        tuiAction
	width         int
	height        int
	maxBranchLen  int
}

type createResultMsg struct {
	err error
}

type deleteResultMsg struct {
	err error
}

type branchesResultMsg struct {
	branches []string
	err      error
}

func runTUI() (tuiAction, error) {
	repoRoot, err := gitRepoRoot()
	if err != nil {
		return tuiAction{}, err
	}

	model, err := newTUIModel(repoRoot)
	if err != nil {
		return tuiAction{}, err
	}

	p := newProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		if errors.Is(err, tea.ErrProgramKilled) {
			return tuiAction{}, nil
		}
		return tuiAction{}, err
	}
	return finalModel.(tuiModel).action, nil
}

func newTUIModel(repoRoot string) (tuiModel, error) {
	wts, err := gitWorktrees(repoRoot)
	if err != nil {
		return tuiModel{}, err
	}
	if len(wts) == 0 {
		return tuiModel{}, errors.New("no worktrees found")
	}
	mainWT := wts[0].Path
	items, maxLen := buildWorktreeItems(wts)
	l := newListModel("Worktrees", items)

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	return tuiModel{
		state:        tuiStateList,
		repoRoot:     repoRoot,
		mainWorktree: mainWT,
		list:         l,
		copyConfig:   true,
		spinner:      spin,
		maxBranchLen: maxLen,
	}, nil
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		innerW := msg.Width - 2 // frame border left + right
		switch m.state {
		case tuiStateList:
			// Reserve: frame(2) + title(1) + column header(1) + footer(1) + status(1)
			innerH := msg.Height - 6
			if nItems := len(m.list.Items()); nItems+2 < innerH {
				innerH = nItems + 2
			}
			m.list.SetSize(innerW, innerH)
		case tuiStateNewBranch:
			// Reserve: frame(2) + title(1) + footer(1) + status(1)
			innerH := msg.Height - 5
			if nItems := len(m.branches.Items()); nItems+2 < innerH {
				innerH = nItems + 2
			}
			m.branches.SetSize(innerW, innerH)
		}
	case tea.KeyMsg:
		if m.state == tuiStateBusy {
			return m, nil
		}
		switch msg.String() {
		case "q":
			if m.isFiltering() || m.state == tuiStateInputBranchName {
				break
			}
			m.action = tuiAction{kind: tuiActionNone}
			return m, tea.Quit
		case "ctrl+c":
			m.action = tuiAction{kind: tuiActionNone}
			return m, tea.Quit
		}
	case spinner.TickMsg:
		if m.state == tuiStateBusy {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case createResultMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			_ = m.reloadWorktrees()
			m.status = "worktree created"
		}
		m.state = tuiStateList
		m.busyText = ""
		return m, nil
	case deleteResultMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			_ = m.reloadWorktrees()
			m.status = "worktree removed"
		}
		m.state = tuiStateList
		m.busyText = ""
		return m, nil
	case branchesResultMsg:
		m.busyText = ""
		if msg.err != nil {
			m.status = msg.err.Error()
			m.state = tuiStateList
			return m, nil
		}
		if len(msg.branches) == 0 {
			m.status = "no branches found"
			m.state = tuiStateList
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.branches))
		for _, branch := range msg.branches {
			items = append(items, branchItem(branch))
		}
		m.branches = newListModel("Select branch", items)
		if m.width > 0 && m.height > 0 {
			innerH := m.height - 5
			if nItems := len(msg.branches); nItems+2 < innerH {
				innerH = nItems + 2
			}
			m.branches.SetSize(m.width-2, innerH)
		}
		m.state = tuiStateNewBranch
		m.status = ""
		return m, nil
	}

	switch m.state {
	case tuiStateList:
		return m.updateList(msg)
	case tuiStateNewBranch:
		return m.updateBranchList(msg)
	case tuiStatePromptConfig:
		return m.updatePromptConfig(msg)
	case tuiStatePromptLibs:
		return m.updatePromptLibs(msg)
	case tuiStateConfirmDelete:
		return m.updateConfirmDelete(msg)
	case tuiStateInputBranchName:
		return m.updateInputBranchName(msg)
	case tuiStateConfirmNewBranch:
		return m.updateConfirmNewBranch(msg)
	case tuiStateHelp:
		return m.updateHelp(msg)
	case tuiStateBusy:
		return m, nil
	default:
		return m, nil
	}
}

func (m tuiModel) View() string {
	switch m.state {
	case tuiStateList:
		return renderFramed(m.listContent(), listFooter(m.width), m.status, m.width)
	case tuiStateNewBranch:
		title := titleStyle.Render("Select branch")
		content := title + "\n" + m.branches.View()
		return renderFramed(content, branchFooter(m.width), m.status, m.width)
	case tuiStatePromptConfig:
		return promptView("Copy config files?", true, m.status, m.width)
	case tuiStatePromptLibs:
		return promptView("Copy libs (node_modules)?", false, m.status, m.width)
	case tuiStateConfirmDelete:
		name := m.pendingDelete.branch
		if name == "" {
			name = filepath.Base(m.pendingDelete.path)
		}
		return promptView(fmt.Sprintf("Remove worktree %q?", name), false, m.status, m.width)
	case tuiStateInputBranchName:
		prompt := fmt.Sprintf("New branch name (from %s):", m.baseBranch)
		content := prompt + "\n" + m.input.View()
		return renderFramed(content, "enter: confirm  esc: back", m.status, m.width)
	case tuiStateConfirmNewBranch:
		prompt := fmt.Sprintf("Create new branch %s from %s?", m.pendingBranch, m.baseBranch)
		return promptView(prompt, true, m.status, m.width)
	case tuiStateBusy:
		status := fmt.Sprintf("%s %s", m.spinner.View(), m.busyText)
		return renderFramed(m.listContent(), listFooter(m.width), status, m.width)
	case tuiStateHelp:
		return renderFramed(helpContent(), "press any key to close", "", m.width)
	default:
		return ""
	}
}

func (m tuiModel) listContent() string {
	title := titleStyle.Render("Worktrees")
	listView := m.list.View()
	header := columnHeader(m.maxBranchLen)
	// Insert column header right before list items. Find the status bar
	// line (ends with "item" or "items") and replace the blank line after
	// it with the column header. This works in both Unfiltered and
	// Filtering states.
	lines := strings.Split(listView, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(ansi.Strip(line))
		if strings.HasSuffix(trimmed, "item") || strings.HasSuffix(trimmed, "items") {
			if i+1 < len(lines) {
				lines[i+1] = header
			}
			break
		}
	}
	return title + "\n" + strings.Join(lines, "\n")
}

func columnHeader(maxBranchLen int) string {
	if maxBranchLen < 6 {
		maxBranchLen = 6
	}
	return headerStyle.Render(fmt.Sprintf("  %-*s  %s", maxBranchLen, "Branch", "Path"))
}

func renderFramed(content, help, status string, width int) string {
	style := frameStyle
	if width > 2 {
		style = style.Width(width - 2)
	}
	framed := style.Render(content)
	if help != "" {
		framed += "\n " + help
	}
	if status != "" {
		framed += "\n " + status
	}
	return framed
}

func (m tuiModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.list.FilterState() != list.Filtering {
			switch keyMsg.String() {
			case "enter":
				item := selectedWorktree(m.list)
				if item.path != "" {
					m.action = tuiAction{kind: tuiActionGo, path: item.path}
					return m, tea.Quit
				}
			case "t":
				item := selectedWorktree(m.list)
				if item.path != "" {
					m.action = tuiAction{kind: tuiActionTmux, path: item.path}
					return m, tea.Quit
				}
			case "n":
				m.state = tuiStateBusy
				m.busyText = "loading branches..."
				m.status = ""
				return m, tea.Batch(m.spinner.Tick, loadBranchesCmd(m.repoRoot))
			case "d":
				item := selectedWorktree(m.list)
				if item.path == "" {
					return m, nil
				}
				clean, err := gitWorktreeClean(item.path)
				if err != nil {
					m.status = err.Error()
					return m, nil
				}
				if !clean {
					m.status = "worktree has uncommitted changes"
					return m, nil
				}
				m.pendingDelete = item
				m.state = tuiStateConfirmDelete
				m.status = ""
				return m, nil
			case "?":
				m.state = tuiStateHelp
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m tuiModel) updateBranchList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if m.branches.FilterState() != list.Filtering {
			switch keyMsg.String() {
			case "esc":
				m.state = tuiStateList
				return m, nil
			case "enter":
				if item, ok := m.branches.SelectedItem().(branchItem); ok {
					m.pendingBranch = string(item)
					m.baseBranch = ""
					m.copyConfig = true
					m.copyLibs = false
					m.state = tuiStatePromptConfig
					m.status = ""
					return m, nil
				}
			case "c":
				if item, ok := m.branches.SelectedItem().(branchItem); ok {
					m.baseBranch = string(item)
					ti := textinput.New()
					ti.Placeholder = "branch-name"
					ti.Focus()
					m.input = ti
					m.state = tuiStateInputBranchName
					m.status = ""
					return m, nil
				}
			case "?":
				m.state = tuiStateHelp
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.branches, cmd = m.branches.Update(msg)
	return m, cmd
}

func (m tuiModel) updatePromptConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y", "enter":
		m.copyConfig = true
		m.state = tuiStatePromptLibs
	case "n", "N":
		m.copyConfig = false
		m.state = tuiStatePromptLibs
	case "esc":
		m.state = tuiStateList
	}
	return m, nil
}

func (m tuiModel) updatePromptLibs(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		m.copyLibs = true
		return m.startCreate()
	case "n", "N", "enter":
		m.copyLibs = false
		return m.startCreate()
	case "esc":
		m.state = tuiStateList
	}
	return m, nil
}

func (m tuiModel) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		return m.startDelete()
	case "n", "N", "esc", "enter":
		m.pendingDelete = worktreeItem{}
		m.state = tuiStateList
	}
	return m, nil
}

func (m tuiModel) updateInputBranchName(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "enter":
		name := strings.TrimSpace(m.input.Value())
		if name == "" {
			return m, nil
		}
		m.pendingBranch = name
		m.state = tuiStateConfirmNewBranch
		return m, nil
	case "esc":
		m.baseBranch = ""
		m.state = tuiStateNewBranch
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m tuiModel) updateConfirmNewBranch(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y", "enter":
		m.copyConfig = true
		m.copyLibs = false
		m.state = tuiStatePromptConfig
		m.status = ""
	case "n", "N", "esc":
		m.baseBranch = ""
		m.pendingBranch = ""
		m.state = tuiStateNewBranch
	}
	return m, nil
}

func (m tuiModel) startCreate() (tea.Model, tea.Cmd) {
	m.state = tuiStateBusy
	m.busyText = "creating worktree..."
	m.pendingDelete = worktreeItem{}
	return m, tea.Batch(m.spinner.Tick, createWorktreeCmd(m))
}

func (m tuiModel) startDelete() (tea.Model, tea.Cmd) {
	m.state = tuiStateBusy
	m.busyText = "removing worktree..."
	return m, tea.Batch(m.spinner.Tick, deleteWorktreeCmd(m))
}

func (m tuiModel) createWorktree() error {
	branch := strings.TrimSpace(m.pendingBranch)
	_, err := addWorktree(m.repoRoot, m.mainWorktree, branch, m.baseBranch, m.copyConfig, m.copyLibs)
	return err
}

func (m *tuiModel) reloadWorktrees() error {
	wts, err := gitWorktrees(m.repoRoot)
	if err != nil {
		return err
	}
	items, maxLen := buildWorktreeItems(wts)
	m.list.SetItems(items)
	m.maxBranchLen = maxLen
	if m.width > 0 && m.height > 0 {
		innerH := m.height - 6
		if nItems := len(items); nItems+2 < innerH {
			innerH = nItems + 2
		}
		m.list.SetSize(m.width-2, innerH)
	}
	return nil
}

func selectedWorktree(m list.Model) worktreeItem {
	item, ok := m.SelectedItem().(worktreeItem)
	if !ok {
		return worktreeItem{}
	}
	return item
}

func buildWorktreeItems(wts []worktree) ([]list.Item, int) {
	maxName := 0
	names := make([]string, 0, len(wts))
	for _, wt := range wts {
		name := wt.Branch
		if name == "" {
			name = filepath.Base(wt.Path)
		}
		names = append(names, name)
		if len(name) > maxName {
			maxName = len(name)
		}
	}

	items := make([]list.Item, 0, len(wts))
	for i, wt := range wts {
		name := names[i]
		padded := fmt.Sprintf("%-*s  %s", maxName, name, wt.Path)
		items = append(items, worktreeItem{
			branch:  wt.Branch,
			path:    wt.Path,
			display: padded,
		})
	}
	return items, maxName
}

func createWorktreeCmd(m tuiModel) tea.Cmd {
	return func() tea.Msg {
		return createResultMsg{err: m.createWorktree()}
	}
}

func deleteWorktreeCmd(m tuiModel) tea.Cmd {
	path := m.pendingDelete.path
	repoRoot := m.repoRoot
	return func() tea.Msg {
		return deleteResultMsg{err: removeWorktree(repoRoot, path)}
	}
}

func newListModel(title string, items []list.Item) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.ShowDescription = false

	l := list.New(items, denseDelegate{DefaultDelegate: delegate}, 0, 0)
	l.Title = title
	l.SetShowHelp(false)
	l.SetShowStatusBar(true)
	l.SetShowTitle(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.Filter = exactMatchFilter
	l.SetSize(80, 20) // fallback size until we receive a WindowSizeMsg
	return l
}

func promptView(prompt string, defaultYes bool, status string, width int) string {
	choice := "[y/N]"
	if defaultYes {
		choice = "[Y/n]"
	}
	content := fmt.Sprintf("%s %s", prompt, choice)
	return renderFramed(content, "enter: accept default  esc: cancel", status, width)
}

func withFooter(body, footer, status string) string {
	output := body
	if footer != "" {
		output += "\n\n" + footer
	}
	return withStatus(output, status)
}

func withStatus(body, status string) string {
	if status == "" {
		return body
	}
	return body + "\n\n" + status
}

func listFooter(width int) string {
	full := "enter: go  t: tmux  n: new  d: delete  /: filter  ?: help  q: quit"
	if width > 0 && width < len(full)+2 {
		return "↵:go t:tmux n:new d:del /:filter ?:help q:quit"
	}
	return full
}

func branchFooter(width int) string {
	full := "enter: select  c: create  esc: back  /: filter  ?: help"
	if width > 0 && width < len(full)+2 {
		return "↵:select c:create esc:back /:filter ?:help"
	}
	return full
}

const listEllipsis = "..."

type denseDelegate struct {
	list.DefaultDelegate
}

func (d denseDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	var (
		title, desc  string
		matchedRunes []int
		s            = &d.Styles
	)

	i, ok := item.(list.DefaultItem)
	if !ok {
		return
	}
	title = i.Title()
	desc = i.Description()

	if m.Width() <= 0 {
		return
	}

	textWidth := m.Width() - s.NormalTitle.GetPaddingLeft() - s.NormalTitle.GetPaddingRight()
	title = ansi.Truncate(title, textWidth, listEllipsis)
	if d.ShowDescription {
		var lines []string
		for i, line := range strings.Split(desc, "\n") {
			if i >= d.Height()-1 {
				break
			}
			lines = append(lines, ansi.Truncate(line, textWidth, listEllipsis))
		}
		desc = strings.Join(lines, "\n")
	}

	isSelected := index == m.Index()
	isFiltered := m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied

	if isFiltered && index < len(m.VisibleItems()) {
		matchedRunes = m.MatchesForItem(index)
	}

	if isSelected {
		if isFiltered {
			unmatched := s.SelectedTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = s.SelectedTitle.Render(title)
		desc = s.SelectedDesc.Render(desc)
	} else {
		if isFiltered {
			unmatched := s.NormalTitle.Inline(true)
			matched := unmatched.Inherit(s.FilterMatch)
			title = lipgloss.StyleRunes(title, matchedRunes, matched, unmatched)
		}
		title = s.NormalTitle.Render(title)
		desc = s.NormalDesc.Render(desc)
	}

	if d.ShowDescription {
		fmt.Fprintf(w, "%s\n%s", title, desc) //nolint:errcheck
		return
	}
	fmt.Fprintf(w, "%s", title) //nolint:errcheck
}

func (m tuiModel) isFiltering() bool {
	switch m.state {
	case tuiStateList:
		return m.list.FilterState() == list.Filtering
	case tuiStateNewBranch:
		return m.branches.FilterState() == list.Filtering
	}
	return false
}

func (m tuiModel) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.state = tuiStateList
		return m, nil
	}
	return m, nil
}

func helpContent() string {
	return titleStyle.Render("Keyboard Shortcuts") + "\n\n" +
		"  Worktree List\n" +
		"  enter    Open shell in worktree\n" +
		"  t        Open tmux session\n" +
		"  n        Create new worktree\n" +
		"  d        Delete worktree\n" +
		"  /        Filter list\n" +
		"  j/k      Navigate up/down\n" +
		"  ?        Show this help\n" +
		"  q        Quit\n\n" +
		"  Branch Selection\n" +
		"  enter    Select branch\n" +
		"  c        Create new branch\n" +
		"  /        Filter branches\n" +
		"  esc      Go back"
}

func loadBranchesCmd(repoRoot string) tea.Cmd {
	return func() tea.Msg {
		branches, err := gitBranches(repoRoot)
		if err != nil {
			return branchesResultMsg{err: err}
		}
		ordered := orderByRecentCommit(branches, repoRoot, "branches")
		return branchesResultMsg{branches: ordered}
	}
}

func exactMatchFilter(term string, targets []string) []list.Rank {
	term = strings.TrimSpace(term)
	if term == "" {
		ranks := make([]list.Rank, len(targets))
		for i := range targets {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}

	lowerTerm := strings.ToLower(term)
	var ranks []list.Rank
	for i, target := range targets {
		lowerTarget := strings.ToLower(target)
		start := strings.Index(lowerTarget, lowerTerm)
		if start == -1 {
			continue
		}
		matches := make([]int, 0, len(term))
		for j := 0; j < len(term); j++ {
			matches = append(matches, start+j)
		}
		ranks = append(ranks, list.Rank{Index: i, MatchedIndexes: matches})
	}
	return ranks
}
