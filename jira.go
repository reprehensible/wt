package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	osGetenv    = os.Getenv
	osWriteFile = os.WriteFile
	jiraGet     = jiraGetDefault
)

type jiraIssue struct {
	Key    string     `json:"key"`
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Comment     jiraComments `json:"comment"`
}

type jiraComments struct {
	Comments []jiraComment `json:"comments"`
}

type jiraComment struct {
	Author  jiraAuthor `json:"author"`
	Body    string     `json:"body"`
	Created string     `json:"created"`
}

type jiraAuthor struct {
	DisplayName string `json:"displayName"`
}

func jiraGetDefault(url, user, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized:
		return nil, errors.New("jira: authentication failed (401)")
	case http.StatusNotFound:
		return nil, errors.New("jira: issue not found (404)")
	default:
		return nil, fmt.Errorf("jira: unexpected status %d", resp.StatusCode)
	}
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string, maxLen int) string {
	s = strings.ToLower(s)
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen]
		// Truncate at last hyphen to avoid partial words
		if i := strings.LastIndex(s, "-"); i > 0 {
			s = s[:i]
		}
	}

	return s
}

func jiraBranchName(key, summary string) string {
	if summary == "" {
		return key
	}
	slug := slugify(summary, 50)
	if slug == "" {
		return key
	}
	return key + "-" + slug
}

func renderIssueMD(issue jiraIssue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n", issue.Key, issue.Fields.Summary)

	if issue.Fields.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", issue.Fields.Description)
	}

	if len(issue.Fields.Comment.Comments) > 0 {
		fmt.Fprintf(&b, "\n## Comments\n")
		for _, c := range issue.Fields.Comment.Comments {
			fmt.Fprintf(&b, "\n### %s (%s)\n\n%s\n", c.Author.DisplayName, c.Created, c.Body)
		}
	}

	return b.String()
}

func jiraCmd(args []string) {
	fs := flag.NewFlagSet("jira", flag.ExitOnError)
	tmux := fs.Bool("t", false, "open worktree in tmux after creation")
	branch := fs.String("branch", "", "override branch name")
	fs.StringVar(branch, "b", "", "override branch name")
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

	issueKey := ""
	if fs.NArg() > 0 {
		issueKey = fs.Arg(0)
	}
	if issueKey == "" {
		die(errors.New("issue key required (e.g. PROJ-123)"))
	}

	jiraURL := osGetenv("JIRA_URL")
	jiraUser := osGetenv("JIRA_USER")
	jiraToken := osGetenv("JIRA_TOKEN")

	if jiraURL == "" || jiraUser == "" || jiraToken == "" {
		die(errors.New("JIRA_URL, JIRA_USER, and JIRA_TOKEN must be set"))
	}

	jiraURL = strings.TrimRight(jiraURL, "/")
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=summary,description,comment", jiraURL, issueKey)

	body, err := jiraGet(apiURL, jiraUser, jiraToken)
	if err != nil {
		die(err)
	}

	var issue jiraIssue
	if err := json.Unmarshal(body, &issue); err != nil {
		die(fmt.Errorf("jira: invalid response: %w", err))
	}

	branchName := *branch
	if branchName == "" {
		branchName = jiraBranchName(issue.Key, issue.Fields.Summary)
	}

	if *noCopyConfig {
		*copyConfig = false
	}
	if *noCopyLibs {
		*copyLibs = false
	}

	wtPath, err := addWorktree(branchName, *fromBranch, *copyConfig, *copyLibs)
	if err != nil {
		die(err)
	}

	md := renderIssueMD(issue)
	mdPath := filepath.Join(wtPath, issue.Key+".md")
	if err := osWriteFile(mdPath, []byte(md), 0o644); err != nil {
		die(err)
	}

	fmt.Fprintln(stdout, wtPath)

	if *tmux {
		if err := openTmux(wtPath); err != nil {
			die(err)
		}
	}
}
