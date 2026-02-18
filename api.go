package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// addWorktree creates a new git worktree for the given branch.
// repoRoot is the git repository root, mainWT is the main worktree path
// (used as the base for the new worktree path and as the source for file copies).
func addWorktree(repoRoot, mainWT, branch, fromBranch string, copyConfig, copyLibs bool) (string, error) {
	if branch == "" {
		return "", errors.New("branch required")
	}

	wtPath := worktreePath(mainWT, branch)
	if err := osMkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return "", err
	}

	if fromBranch != "" {
		if err := runGit(repoRoot, "worktree", "add", "-b", branch, wtPath, fromBranch); err != nil {
			return "", err
		}
	} else {
		exists, err := gitBranchExists(repoRoot, branch)
		if err != nil {
			return "", err
		}
		if exists {
			if err := runGit(repoRoot, "worktree", "add", wtPath, branch); err != nil {
				return "", err
			}
		} else {
			if err := runGit(repoRoot, "worktree", "add", "-b", branch, wtPath); err != nil {
				return "", err
			}
		}
	}

	if copyConfig {
		if err := copyItems(mainWT, wtPath, defaultCopyConfigItems); err != nil {
			return "", err
		}
		if err := copyMatchingFiles(mainWT, wtPath, defaultCopyConfigRecursive); err != nil {
			return "", err
		}
	}
	if copyLibs {
		if err := copyItems(mainWT, wtPath, defaultCopyLibItems); err != nil {
			return "", err
		}
	}

	return wtPath, nil
}

// findWorktree looks up a worktree by name, matching against branch name,
// directory basename, or full path (in that priority order).
func findWorktree(repoRoot, name string) (string, error) {
	wts, err := gitWorktrees(repoRoot)
	if err != nil {
		return "", err
	}
	if len(wts) == 0 {
		return "", errors.New("no worktrees found")
	}

	for _, wt := range wts {
		if wt.Branch == name {
			return wt.Path, nil
		}
		if filepath.Base(wt.Path) == name {
			return wt.Path, nil
		}
		if wt.Path == name {
			return wt.Path, nil
		}
	}
	return "", fmt.Errorf("worktree not found: %s", name)
}

// removeWorktree removes a git worktree at the given path.
func removeWorktree(repoRoot, path string) error {
	return runGit(repoRoot, "worktree", "remove", path)
}

// openShell opens an interactive shell in the given directory.
func openShell(targetPath string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := execCommand(shell)
	cmd.Dir = targetPath
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// openTmux opens or attaches to a tmux session for the given directory.
func openTmux(targetPath string) error {
	sessionName := filepath.Base(targetPath)

	checkCmd := execCommand("tmux", "has-session", "-t", sessionName)
	sessionExists := checkCmd.Run() == nil

	inTmux := os.Getenv("TMUX") != ""

	if !sessionExists {
		if inTmux {
			cmd := execCommand("tmux", "new-session", "-d", "-s", sessionName, "-c", targetPath)
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				return err
			}
			cmd = execCommand("tmux", "switch-client", "-t", sessionName)
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd.Run()
		}
		cmd := execCommand("tmux", "new-session", "-s", sessionName, "-c", targetPath)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}

	if inTmux {
		cmd := execCommand("tmux", "switch-client", "-t", sessionName)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}
	cmd := execCommand("tmux", "attach-session", "-t", sessionName)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
