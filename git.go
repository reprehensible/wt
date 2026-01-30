package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var execCommand = exec.Command

func runGit(repoRoot string, args ...string) error {
	_, err := runGitOutput(repoRoot, args...)
	return err
}

func runGitOutput(repoRoot string, args ...string) (string, error) {
	cmdArgs := args
	if repoRoot != "" {
		cmdArgs = append([]string{"-C", repoRoot}, args...)
	}
	cmd := execCommand("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func gitRepoRoot() (string, error) {
	out, err := runGitOutput("", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitMainWorktree(repoRoot string) (string, error) {
	wts, err := gitWorktrees(repoRoot)
	if err != nil {
		return "", err
	}
	if len(wts) == 0 {
		return "", errors.New("no worktrees found")
	}
	return wts[0].Path, nil
}

func worktreePath(repoRoot, branch string) string {
	return filepath.Join(repoRoot+"-worktrees", filepath.FromSlash(branch))
}

func gitBranches(repoRoot string) ([]string, error) {
	out, err := runGitOutput(repoRoot, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var branches []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		branches = append(branches, line)
	}
	return branches, nil
}

func gitBranchExists(repoRoot, branch string) (bool, error) {
	_, err := runGitOutput(repoRoot, "show-ref", "--verify", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func gitWorktrees(repoRoot string) ([]worktree, error) {
	out, err := runGitOutput(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out, "\n")
	var wts []worktree
	var current worktree
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				wts = append(wts, current)
				current = worktree{}
			}
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "worktree":
			current.Path = parts[1]
		case "branch":
			current.Branch = strings.TrimPrefix(parts[1], "refs/heads/")
		}
	}
	if current.Path != "" {
		wts = append(wts, current)
	}
	return wts, nil
}

func gitWorktreeClean(path string) (bool, error) {
	out, err := runGitOutput(path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

func gitCommitTime(repoRoot, ref string) int64 {
	out, err := runGitOutput(repoRoot, "log", "-1", "--format=%ct", ref)
	if err != nil {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func gitCommitTimePath(path string) int64 {
	out, err := runGitOutput(path, "log", "-1", "--format=%ct")
	if err != nil {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func orderByRecentCommit(items []string, repoRoot, orderKey string) []string {
	type entry struct {
		name string
		ts   int64
	}

	entries := make([]entry, 0, len(items))
	for _, item := range items {
		var ts int64
		switch orderKey {
		case "branches":
			ts = gitCommitTime(repoRoot, item)
		case "worktrees":
			if filepath.IsAbs(item) {
				ts = gitCommitTimePath(item)
			} else {
				ts = gitCommitTime(repoRoot, item)
			}
		default:
			ts = gitCommitTime(repoRoot, item)
		}
		entries = append(entries, entry{name: item, ts: ts})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ts == entries[j].ts {
			return false
		}
		return entries[i].ts > entries[j].ts
	})

	ordered := make([]string, 0, len(entries))
	for _, entry := range entries {
		ordered = append(ordered, entry.name)
	}
	return ordered
}
