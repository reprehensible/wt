package main

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	stdout   io.Writer = os.Stdout
	stderr   io.Writer = os.Stderr
	stdin    io.Reader = os.Stdin
	exitFunc           = os.Exit

	newCmdFn  = newCmd
	listCmdFn = listCmd
	goCmdFn   = goCmd
	tmuxCmdFn = tmuxCmd
	jiraCmdFn = jiraCmd

	newProgram = func(model tea.Model, opts ...tea.ProgramOption) programRunner {
		return tea.NewProgram(model, opts...)
	}
)

func main() {
	if len(os.Args) < 2 {
		action, err := runTUI()
		if err != nil {
			die(err)
		}
		switch action.kind {
		case tuiActionGo:
			if err := openShell(action.path); err != nil {
				die(err)
			}
		case tuiActionTmux:
			if err := openTmux(action.path); err != nil {
				die(err)
			}
		}
		return
	}

	sub := os.Args[1]
	switch sub {
	case "new":
		newCmdFn(os.Args[2:])
	case "list":
		listCmdFn(os.Args[2:])
	case "go":
		goCmdFn(os.Args[2:])
	case "t":
		tmuxCmdFn(os.Args[2:])
	case "jira":
		jiraCmdFn(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", sub)
		printUsage()
		exitFunc(2)
	}
}
