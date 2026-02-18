package main

import (
	"errors"
	"flag"
	"fmt"
)

func printUsage() {
	fmt.Fprintln(stderr, "wt - manage git worktrees")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "usage: wt [command] [options]")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "commands:")
	fmt.Fprintln(stderr, "  (no command)        open interactive worktree manager")
	fmt.Fprintln(stderr, "  new <branch>        create a new worktree")
	fmt.Fprintln(stderr, "  list                list worktrees")
	fmt.Fprintln(stderr, "  go <name>           enter a worktree shell")
	fmt.Fprintln(stderr, "  t <name>            open worktree in tmux session")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "  jira new <key>      create worktree from Jira issue")
	fmt.Fprintln(stderr, "  jira status [key]   view/update Jira issue status")
	fmt.Fprintln(stderr, "  jira config         show/init status mappings")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Run 'wt <command> --help' for details on a specific command.")
}

func printNewUsage() {
	fmt.Fprintln(stderr, "usage: wt new [options] <branch>")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Create a new worktree for the given branch.")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "options:")
	fmt.Fprintln(stderr, "  -c, --copy-config      copy config files (default: on)")
	fmt.Fprintln(stderr, "  -C, --no-copy-config   skip copying config files")
	fmt.Fprintln(stderr, "  -l, --copy-libs        copy library directories")
	fmt.Fprintln(stderr, "  -L, --no-copy-libs     skip copying libraries (default)")
	fmt.Fprintln(stderr, "  -f, --from <branch>    base branch to create from")
}

func printListUsage() {
	fmt.Fprintln(stderr, "usage: wt list")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "List all worktrees with their branch names and paths.")
}

func printGoUsage() {
	fmt.Fprintln(stderr, "usage: wt go <name>")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Open a shell in the named worktree. Matches against branch")
	fmt.Fprintln(stderr, "names and directory basenames.")
}

func printTmuxUsage() {
	fmt.Fprintln(stderr, "usage: wt t <name>")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Open the named worktree in a tmux session.")
}

func printJiraUsage() {
	fmt.Fprintln(stderr, "usage: wt jira <new|status|config> [options]")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Jira integration for worktree management.")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "subcommands:")
	fmt.Fprintln(stderr, "  new <key>           create worktree from Jira issue")
	fmt.Fprintln(stderr, "  status [key]        view/update Jira issue status")
	fmt.Fprintln(stderr, "  status sync         sync Jira status from GitHub PR state")
	fmt.Fprintln(stderr, "  config              show status mappings")
	fmt.Fprintln(stderr, "  config --init       bootstrap a template config")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "environment variables: JIRA_URL, JIRA_USER, JIRA_TOKEN")
}

func printJiraNewUsage() {
	fmt.Fprintln(stderr, "usage: wt jira new [options] <key>")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Create a worktree from a Jira issue. The branch name is")
	fmt.Fprintln(stderr, "generated from the issue key and summary.")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "options:")
	fmt.Fprintln(stderr, "  -t                     open worktree in tmux after creation")
	fmt.Fprintln(stderr, "  -b, --branch <name>    override auto-generated branch name")
	fmt.Fprintln(stderr, "  -c, --copy-config      copy config files (default: on)")
	fmt.Fprintln(stderr, "  -C, --no-copy-config   skip copying config files")
	fmt.Fprintln(stderr, "  -l, --copy-libs        copy library directories")
	fmt.Fprintln(stderr, "  -L, --no-copy-libs     skip copying libraries (default)")
	fmt.Fprintln(stderr, "  -f, --from <branch>    base branch to create from")
	fmt.Fprintln(stderr, "  -S, --no-status-update skip auto-transition to working")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "environment variables: JIRA_URL, JIRA_USER, JIRA_TOKEN")
}

func printJiraStatusUsage() {
	fmt.Fprintln(stderr, "usage: wt jira status [key] [status]")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "View or update a Jira issue's status. If no key is given,")
	fmt.Fprintln(stderr, "the issue key is inferred from the current branch name.")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "subcommands:")
	fmt.Fprintln(stderr, "  sync                sync status from GitHub PR state")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "sync options:")
	fmt.Fprintln(stderr, "  -n, --dry-run       show what would happen without making changes")
}

func printJiraConfigUsage() {
	fmt.Fprintln(stderr, "usage: wt jira config [--init]")
	fmt.Fprintln(stderr, "")
	fmt.Fprintln(stderr, "Show current Jira status mappings, or bootstrap a template")
	fmt.Fprintln(stderr, "config file with --init.")
}

func newCmd(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	fs.Usage = printNewUsage
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
		fmt.Fprintln(stderr, "error: branch required")
		fmt.Fprintln(stderr, "")
		printNewUsage()
		exitFunc(1)
		return
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
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			printListUsage()
			return
		}
	}
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
	fs.Usage = printGoUsage
	_ = fs.Parse(args)

	name := ""
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		fmt.Fprintln(stderr, "error: worktree name required")
		fmt.Fprintln(stderr, "")
		printGoUsage()
		exitFunc(1)
		return
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
	fs.Usage = printTmuxUsage
	_ = fs.Parse(args)

	name := ""
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		fmt.Fprintln(stderr, "error: worktree name required")
		fmt.Fprintln(stderr, "")
		printTmuxUsage()
		exitFunc(1)
		return
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
