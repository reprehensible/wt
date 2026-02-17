package main

import (
	"bytes"
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
	jiraPost    = jiraPostDefault
)

type jiraIssue struct {
	Key    string     `json:"key"`
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Comment     jiraComments `json:"comment"`
	Status      jiraStatus   `json:"status"`
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

type jiraStatus struct {
	Name string `json:"name"`
}

type jiraTransition struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	To   jiraStatus `json:"to"`
}

type jiraTransitionsResponse struct {
	Transitions []jiraTransition `json:"transitions"`
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

func jiraPostDefault(url, user, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira: unexpected status %d", resp.StatusCode)
	}
	return respBody, nil
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

var issueKeyRe = regexp.MustCompile(`^([A-Z]+-\d+)`)

func jiraIssueKeyFromBranch(branch string) string {
	m := issueKeyRe.FindStringSubmatch(branch)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func jiraEnv() (string, string, string, error) {
	jiraURL := osGetenv("JIRA_URL")
	jiraUser := osGetenv("JIRA_USER")
	jiraToken := osGetenv("JIRA_TOKEN")
	if jiraURL == "" || jiraUser == "" || jiraToken == "" {
		return "", "", "", errors.New("JIRA_URL, JIRA_USER, and JIRA_TOKEN must be set")
	}
	return strings.TrimRight(jiraURL, "/"), jiraUser, jiraToken, nil
}

func jiraFetchIssue(baseURL, issueKey, user, token string) (jiraIssue, error) {
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=summary,description,comment,status", baseURL, issueKey)
	body, err := jiraGet(apiURL, user, token)
	if err != nil {
		return jiraIssue{}, err
	}
	var issue jiraIssue
	if err := json.Unmarshal(body, &issue); err != nil {
		return jiraIssue{}, fmt.Errorf("jira: invalid response: %w", err)
	}
	return issue, nil
}

func jiraSetStatus(baseURL, issueKey, statusName, user, token string) error {
	tURL := fmt.Sprintf("%s/rest/api/2/issue/%s/transitions", baseURL, issueKey)
	body, err := jiraGet(tURL, user, token)
	if err != nil {
		return err
	}
	var tr jiraTransitionsResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return fmt.Errorf("jira: invalid transitions response: %w", err)
	}
	for _, t := range tr.Transitions {
		if strings.EqualFold(t.To.Name, statusName) {
			payload, _ := json.Marshal(map[string]any{
				"transition": map[string]string{"id": t.ID},
			})
			_, err := jiraPost(tURL, user, token, payload)
			return err
		}
	}
	return fmt.Errorf("jira: no transition to %q available", statusName)
}

func jiraCmd(args []string) {
	if len(args) == 0 {
		die(errors.New("usage: wt jira <new|status> ..."))
	}
	switch args[0] {
	case "new":
		jiraNewCmd(args[1:])
	case "status":
		jiraStatusCmd(args[1:])
	default:
		die(fmt.Errorf("unknown jira command: %s", args[0]))
	}
}

func jiraNewCmd(args []string) {
	fs := flag.NewFlagSet("jira new", flag.ExitOnError)
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

	baseURL, user, token, err := jiraEnv()
	if err != nil {
		die(err)
	}

	issue, err := jiraFetchIssue(baseURL, issueKey, user, token)
	if err != nil {
		die(err)
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

func jiraStatusCmd(args []string) {
	issueKey := ""
	statusName := ""

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		issueKey = args[0]
		args = args[1:]
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		statusName = args[0]
	}

	if issueKey == "" {
		branch, err := runGitOutput("", "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			die(fmt.Errorf("jira: could not determine current branch: %w", err))
		}
		issueKey = jiraIssueKeyFromBranch(strings.TrimSpace(branch))
		if issueKey == "" {
			die(errors.New("jira: current branch does not contain an issue key"))
		}
	}

	baseURL, user, token, err := jiraEnv()
	if err != nil {
		die(err)
	}

	if statusName != "" {
		if err := jiraSetStatus(baseURL, issueKey, statusName, user, token); err != nil {
			die(err)
		}
		fmt.Fprintf(stdout, "%s → %s\n", issueKey, statusName)
		return
	}

	issue, err := jiraFetchIssue(baseURL, issueKey, user, token)
	if err != nil {
		die(err)
	}

	fmt.Fprintf(stdout, "%s: %s\n", issue.Key, issue.Fields.Status.Name)

	tURL := fmt.Sprintf("%s/rest/api/2/issue/%s/transitions", baseURL, issueKey)
	body, err := jiraGet(tURL, user, token)
	if err != nil {
		die(err)
	}
	var tr jiraTransitionsResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		die(fmt.Errorf("jira: invalid transitions response: %w", err))
	}
	if len(tr.Transitions) > 0 {
		fmt.Fprintln(stdout, "\nAvailable transitions:")
		for _, t := range tr.Transitions {
			fmt.Fprintf(stdout, "  %s\n", t.To.Name)
		}
	}
}
