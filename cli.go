package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func printUsage() {
	fmt.Fprintln(stderr, "usage: wt <command> [options]")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "commands:")
	fmt.Fprintln(stderr, "  (no command)    open interactive worktree manager")
	fmt.Fprintln(stderr, "  new [branch]    create a new worktree")
	fmt.Fprintln(stderr, "  list            list worktrees")
	fmt.Fprintln(stderr, "  go [name]        enter a worktree shell")
	fmt.Fprintln(stderr, "  t [name]         open worktree in tmux session")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "new options:")
	fmt.Fprintln(stderr, "  -c, --copy-config     copy config files (default)")
	fmt.Fprintln(stderr, "  -C, --no-copy-config  skip copying config files")
	fmt.Fprintln(stderr, "  -l, --copy-libs       copy libraries (default: off)")
	fmt.Fprintln(stderr, "  -L, --no-copy-libs    skip copying libraries")
	fmt.Fprintln(stderr, "  -f, --from            base branch to create from")
}

func newCmd(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	copyConfig := fs.Bool("copy-config", true, "copy config files")
	fs.BoolVar(copyConfig, "c", true, "copy config files")
	noCopyConfig := fs.Bool("no-copy-config", false, "skip copying config files")
	fs.BoolVar(noCopyConfig, "C", false, "skip copying config files")
	copyLibs := fs.Bool("copy-libs", false, "copy libraries")
	fs.BoolVar(copyLibs, "l", false, "copy libraries")
	noCopyLibs := fs.Bool("no-copy-libs", false, "skip copying libraries")
	fs.BoolVar(noCopyLibs, "L", false, "skip copying libraries")
	fromBranch := fs.String("from", "", "base branch to create from")
	fs.StringVar(fromBranch, "f", "", "base branch to create from")
	_ = fs.Parse(args)

	branch := ""
	if fs.NArg() > 0 {
		branch = fs.Arg(0)
	}
	if branch == "" {
		die(errors.New("branch required"))
	}

	repoRoot, err := gitRepoRoot()
	if err != nil {
		die(err)
	}

	mainWT, err := gitMainWorktree(repoRoot)
	if err != nil {
		die(err)
	}

	wtPath := worktreePath(mainWT, branch)
	if err := osMkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		die(err)
	}

	if *fromBranch != "" {
		if err := runGit(repoRoot, "worktree", "add", "-b", branch, wtPath, *fromBranch); err != nil {
			die(err)
		}
	} else {
		exists, err := gitBranchExists(repoRoot, branch)
		if err != nil {
			die(err)
		}
		if exists {
			if err := runGit(repoRoot, "worktree", "add", wtPath, branch); err != nil {
				die(err)
			}
		} else {
			if err := runGit(repoRoot, "worktree", "add", "-b", branch, wtPath); err != nil {
				die(err)
			}
		}
	}

	if *noCopyConfig {
		*copyConfig = false
	}
	if *noCopyLibs {
		*copyLibs = false
	}

	if *copyConfig {
		if err := copyItems(mainWT, wtPath, defaultCopyConfigItems); err != nil {
			die(err)
		}
		if err := copyMatchingFiles(mainWT, wtPath, defaultCopyConfigRecursive); err != nil {
			die(err)
		}
	}
	if *copyLibs {
		if err := copyItems(mainWT, wtPath, defaultCopyLibItems); err != nil {
			die(err)
		}
	}

	fmt.Fprintln(stdout, wtPath)
}

func listCmd(args []string) {
	if len(args) > 0 {
		die(errors.New("list does not take arguments"))
	}

	repoRoot, err := gitRepoRoot()
	if err != nil {
		die(err)
	}

	wts, err := gitWorktrees(repoRoot)
	if err != nil {
		die(err)
	}

	for _, wt := range wts {
		if wt.Branch != "" {
			fmt.Fprintf(stdout, "%s\t%s\n", wt.Branch, wt.Path)
			continue
		}
		fmt.Fprintf(stdout, "%s\n", wt.Path)
	}
}

func goCmd(args []string) {
	fs := flag.NewFlagSet("go", flag.ExitOnError)
	_ = fs.Parse(args)

	name := ""
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		die(errors.New("worktree required"))
	}

	repoRoot, err := gitRepoRoot()
	if err != nil {
		die(err)
	}

	wts, err := gitWorktrees(repoRoot)
	if err != nil {
		die(err)
	}
	if len(wts) == 0 {
		die(errors.New("no worktrees found"))
	}

	var targetPath string
	for _, wt := range wts {
		if wt.Branch == name {
			targetPath = wt.Path
			break
		}
		if filepath.Base(wt.Path) == name {
			targetPath = wt.Path
			break
		}
		if wt.Path == name {
			targetPath = wt.Path
			break
		}
	}
	if targetPath == "" {
		die(fmt.Errorf("worktree not found: %s", name))
	}

	if err := openShell(targetPath); err != nil {
		die(err)
	}
}

func tmuxCmd(args []string) {
	fs := flag.NewFlagSet("t", flag.ExitOnError)
	_ = fs.Parse(args)

	name := ""
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		die(errors.New("worktree required"))
	}

	repoRoot, err := gitRepoRoot()
	if err != nil {
		die(err)
	}

	wts, err := gitWorktrees(repoRoot)
	if err != nil {
		die(err)
	}
	if len(wts) == 0 {
		die(errors.New("no worktrees found"))
	}

	var targetPath string
	for _, wt := range wts {
		if wt.Branch == name {
			targetPath = wt.Path
			break
		}
		if filepath.Base(wt.Path) == name {
			targetPath = wt.Path
			break
		}
		if wt.Path == name {
			targetPath = wt.Path
			break
		}
	}
	if targetPath == "" {
		die(fmt.Errorf("worktree not found: %s", name))
	}

	if err := openTmux(targetPath); err != nil {
		die(err)
	}
}

func openTmux(targetPath string) error {
	sessionName := filepath.Base(targetPath)

	// Check if session already exists
	checkCmd := execCommand("tmux", "has-session", "-t", sessionName)
	sessionExists := checkCmd.Run() == nil

	inTmux := os.Getenv("TMUX") != ""

	if !sessionExists {
		// Create a new session
		if inTmux {
			// Create detached, then switch
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
		// Not in tmux: create and attach
		cmd := execCommand("tmux", "new-session", "-s", sessionName, "-c", targetPath)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}

	// Session exists
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

func die(err error) {
	fmt.Fprintln(stderr, err)
	exitFunc(1)
}
