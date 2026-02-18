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
		// Reserve lines for: frame border(2), title(1), help(1), status(1)
		baseOverhead := 5
		switch m.state {
		case tuiStateList:
			m.list.SetSize(innerW, msg.Height-baseOverhead-1) // extra 1 for column header
		case tuiStateNewBranch:
			m.branches.SetSize(innerW, msg.Height-baseOverhead)
		}
	case tea.KeyMsg:
		if m.state == tuiStateBusy {
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
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
	case tuiStateBusy:
		return m, nil
	default:
		return m, nil
	}
}

func (m tuiModel) View() string {
	switch m.state {
	case tuiStateList:
		return renderFramed(m.listContent(), listFooter(), m.status, m.width)
	case tuiStateNewBranch:
		title := titleStyle.Render("Select branch")
		content := title + "\n" + m.branches.View()
		return renderFramed(content, branchFooter(), m.status, m.width)
	case tuiStatePromptConfig:
		return promptView("Copy config files?", true, m.status)
	case tuiStatePromptLibs:
		return promptView("Copy libs (node_modules)?", false, m.status)
	case tuiStateConfirmDelete:
		return promptView("Remove selected worktree?", false, m.status)
	case tuiStateInputBranchName:
		prompt := fmt.Sprintf("New branch name (from %s):", m.baseBranch)
		content := prompt + "\n" + m.input.View()
		return renderFramed(content, "enter: confirm  esc: back", m.status, m.width)
	case tuiStateConfirmNewBranch:
		prompt := fmt.Sprintf("Create new branch %s from %s?", m.pendingBranch, m.baseBranch)
		return promptView(prompt, true, m.status)
	case tuiStateBusy:
		status := fmt.Sprintf("%s %s", m.spinner.View(), m.busyText)
		return renderFramed(m.listContent(), listFooter(), status, m.width)
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
				branches, err := gitBranches(m.repoRoot)
				if err != nil {
					m.status = err.Error()
					return m, nil
				}
				if len(branches) == 0 {
					m.status = "no branches found"
					return m, nil
				}
				ordered := orderByRecentCommit(branches, m.repoRoot, "branches")
				items := make([]list.Item, 0, len(ordered))
				for _, branch := range ordered {
					items = append(items, branchItem(branch))
				}
				m.branches = newListModel("Select branch", items)
				if m.width > 0 {
					m.branches.SetSize(m.width-2, m.height-5)
				}
				m.state = tuiStateNewBranch
				m.status = ""
				return m, nil
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
					return m, nil
				}
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

func promptView(prompt string, defaultYes bool, status string) string {
	choice := "[y/N]"
	if defaultYes {
		choice = "[Y/n]"
	}
	line := fmt.Sprintf("%s %s", prompt, choice)
	return withStatus(line+"\n\n(enter to accept default, esc to cancel)", status)
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

func listFooter() string {
	return "enter: go  t: tmux  n: new  d: delete  /: filter  q: quit"
}

func branchFooter() string {
	return "enter: select  c: create  esc: back  /: filter"
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
