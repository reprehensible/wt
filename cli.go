package main

import (
	"errors"
	"flag"
	"fmt"
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
	fmt.Fprintln(stderr, "  jira new [key]     create worktree from Jira issue")
	fmt.Fprintln(stderr, "  jira status [key]  view/update Jira issue status")
	fmt.Fprintln(stderr, "  jira status sync   sync Jira status from GitHub PR state")
	fmt.Fprintln(stderr, "  jira config [key]  show status mappings")
	fmt.Fprintln(stderr, "  jira config --init bootstrap a template config")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "new options:")
	fmt.Fprintln(stderr, "  -c, --copy-config     copy config files (default)")
	fmt.Fprintln(stderr, "  -C, --no-copy-config  skip copying config files")
	fmt.Fprintln(stderr, "  -l, --copy-libs       copy libraries (default: off)")
	fmt.Fprintln(stderr, "  -L, --no-copy-libs    skip copying libraries")
	fmt.Fprintln(stderr, "  -f, --from            base branch to create from")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "jira new options:")
	fmt.Fprintln(stderr, "  -t                    open worktree in tmux after creation")
	fmt.Fprintln(stderr, "  -b, --branch          override auto-generated branch name")
	fmt.Fprintln(stderr, "  -c, --copy-config     copy config files (default)")
	fmt.Fprintln(stderr, "  -C, --no-copy-config  skip copying config files")
	fmt.Fprintln(stderr, "  -l, --copy-libs       copy libraries (default: off)")
	fmt.Fprintln(stderr, "  -L, --no-copy-libs    skip copying libraries")
	fmt.Fprintln(stderr, "  -f, --from            base branch to create from")
	fmt.Fprintln(stderr, "  -S, --no-status-update skip auto-transition to working")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "jira status sync options:")
	fmt.Fprintln(stderr, "  -n, --dry-run         show what would happen without making changes")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "jira env vars: JIRA_URL, JIRA_USER, JIRA_TOKEN")
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

	if *noCopyConfig {
		*copyConfig = false
	}
	if *noCopyLibs {
		*copyLibs = false
	}

	repoRoot, err := gitRepoRoot()
	if err != nil {
		die(err)
	}
	mainWT, err := gitMainWorktree(repoRoot)
	if err != nil {
		die(err)
	}

	wtPath, err := addWorktree(repoRoot, mainWT, branch, *fromBranch, *copyConfig, *copyLibs)
	if err != nil {
		die(err)
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

	targetPath, err := findWorktree(repoRoot, name)
	if err != nil {
		die(err)
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

	targetPath, err := findWorktree(repoRoot, name)
	if err != nil {
		die(err)
	}

	if err := openTmux(targetPath); err != nil {
		die(err)
	}
}

func die(err error) {
	fmt.Fprintln(stderr, err)
	exitFunc(1)
}
