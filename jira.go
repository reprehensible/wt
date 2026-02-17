package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

type jiraIssueType struct {
	Name string `json:"name"`
}

type jiraFields struct {
	Summary     string         `json:"summary"`
	Description string         `json:"description"`
	Comment     jiraComments   `json:"comment"`
	Status      jiraStatus     `json:"status"`
	IssueType   jiraIssueType  `json:"issuetype"`
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
	apiURL := fmt.Sprintf("%s/rest/api/2/issue/%s?fields=summary,description,comment,status,issuetype", baseURL, issueKey)
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
		die(errors.New("usage: wt jira <new|status|config> ..."))
	}
	switch args[0] {
	case "new":
		jiraNewCmd(args[1:])
	case "status":
		jiraStatusCmd(args[1:])
	case "config":
		jiraConfigCmd(args[1:])
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
	noStatusUpdate := fs.Bool("no-status-update", false, "skip auto-transition")
	fs.BoolVar(noStatusUpdate, "S", false, "skip auto-transition")
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

	if !*noStatusUpdate {
		cfg, err := loadConfig()
		if err != nil {
			fmt.Fprintf(stderr, "warning: config: %v\n", err)
		} else if !hasStatusConfig(cfg) {
			die(errors.New("no jira status mappings configured; run 'wt jira config --init'"))
		} else {
			target, err := resolveStatus(cfg, issue.Fields.IssueType.Name, "working")
			if err == nil {
				if err := jiraSetStatus(baseURL, issueKey, target, user, token); err != nil {
					fmt.Fprintf(stderr, "warning: %v\n", err)
				} else {
					fmt.Fprintf(stdout, "%s → %s\n", issueKey, target)
				}
			}
		}
	}

	if *tmux {
		if err := openTmux(wtPath); err != nil {
			die(err)
		}
	}
}

func jiraStatusCmd(args []string) {
	if len(args) > 0 && args[0] == "sync" {
		jiraStatusSyncCmd(args[1:])
		return
	}

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

	cfg, cfgErr := loadConfig()

	if len(tr.Transitions) > 0 {
		fmt.Fprintln(stdout, "\nAvailable transitions:")
		for _, t := range tr.Transitions {
			sym := ""
			if cfgErr == nil && hasStatusConfig(cfg) {
				sym = reverseSymbolic(cfg, issue.Fields.IssueType.Name, t.To.Name)
			}
			if sym != "" {
				fmt.Fprintf(stdout, "  %s (%s)\n", t.To.Name, sym)
			} else {
				fmt.Fprintf(stdout, "  %s\n", t.To.Name)
			}
		}
	}

	if cfgErr == nil && !hasStatusConfig(cfg) {
		fmt.Fprintln(stderr, "hint: run 'wt jira config --init' to enable symbolic annotations")
	}
}

func jiraStatusSyncCmd(args []string) {
	fs := flag.NewFlagSet("jira status sync", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would happen without making changes")
	fs.BoolVar(dryRun, "n", false, "dry run")
	_ = fs.Parse(args)

	issueKey := ""
	if fs.NArg() > 0 {
		issueKey = fs.Arg(0)
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

	cfg, err := loadConfig()
	if err != nil {
		die(err)
	}
	if !hasStatusConfig(cfg) {
		die(errors.New("no jira status mappings configured; run 'wt jira config --init'"))
	}

	symbolic, err := ghPRSymbolicStatus()
	if err != nil {
		die(err)
	}
	if symbolic == "" {
		return
	}

	baseURL, user, token, err := jiraEnv()
	if err != nil {
		die(err)
	}

	issue, err := jiraFetchIssue(baseURL, issueKey, user, token)
	if err != nil {
		die(err)
	}

	target, err := resolveStatus(cfg, issue.Fields.IssueType.Name, symbolic)
	if err != nil {
		die(err)
	}

	if strings.EqualFold(issue.Fields.Status.Name, target) {
		fmt.Fprintf(stdout, "%s: already %s\n", issueKey, target)
		return
	}

	if *dryRun {
		fmt.Fprintf(stdout, "%s: %s → %s (dry run)\n", issueKey, issue.Fields.Status.Name, target)
		return
	}

	if err := jiraSetStatus(baseURL, issueKey, target, user, token); err != nil {
		die(err)
	}
	fmt.Fprintf(stdout, "%s → %s\n", issueKey, target)
}

func jiraConfigCmd(args []string) {
	fs := flag.NewFlagSet("jira config", flag.ExitOnError)
	initFlag := fs.Bool("init", false, "bootstrap a template config")
	_ = fs.Parse(args)

	if *initFlag {
		jiraConfigInit()
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		die(err)
	}
	if len(cfg.Jira.Status.Default) == 0 && len(cfg.Jira.Status.Types) == 0 {
		fmt.Fprintln(stdout, "no config found")
		return
	}

	if len(cfg.Jira.Status.Default) > 0 {
		fmt.Fprintln(stdout, "default:")
		keys := make([]string, 0, len(cfg.Jira.Status.Default))
		for k := range cfg.Jira.Status.Default {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(stdout, "  %s → %s\n", k, cfg.Jira.Status.Default[k])
		}
	}

	typeNames := make([]string, 0, len(cfg.Jira.Status.Types))
	for tn := range cfg.Jira.Status.Types {
		typeNames = append(typeNames, tn)
	}
	sort.Strings(typeNames)
	for _, tn := range typeNames {
		m := cfg.Jira.Status.Types[tn]
		if len(m) == 0 {
			continue
		}
		fmt.Fprintf(stdout, "\n%s:\n", tn)
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(stdout, "  %s → %s\n", k, m[k])
		}
	}

	if fs.NArg() > 0 {
		issueKey := fs.Arg(0)
		baseURL, user, token, err := jiraEnv()
		if err != nil {
			die(err)
		}
		issue, err := jiraFetchIssue(baseURL, issueKey, user, token)
		if err != nil {
			die(err)
		}
		issueType := issue.Fields.IssueType.Name

		symbolics := []string{"working", "review", "testing", "done"}
		fmt.Fprintf(stdout, "\nresolved (%s):\n", strings.ToLower(issueType))
		for _, sym := range symbolics {
			target, err := resolveStatus(cfg, issueType, sym)
			if err == nil {
				fmt.Fprintf(stdout, "  %s → %s\n", sym, target)
			}
		}
	}
}

func jiraConfigInit() {
	fmt.Fprintln(stdout, "Where should the config be written?")
	fmt.Fprintln(stdout, "  [g] global  (~/.config/wt/config.json)")
	fmt.Fprintln(stdout, "  [r] repo    (.wt.json)")
	fmt.Fprintf(stdout, "choice [g/r]: ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		die(errors.New("no input"))
	}
	choice := strings.TrimSpace(scanner.Text())

	cfg := templateConfig()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	data = append(data, '\n')

	var path string
	switch choice {
	case "g":
		home, err := osUserHomeDir()
		if err != nil {
			die(err)
		}
		dir := filepath.Join(home, ".config", "wt")
		if err := osMkdirAll(dir, 0o755); err != nil {
			die(err)
		}
		path = filepath.Join(dir, "config.json")
	case "r":
		root, err := gitRepoRoot()
		if err != nil {
			die(err)
		}
		path = filepath.Join(root, ".wt.json")
	default:
		die(fmt.Errorf("invalid choice: %q", choice))
	}

	if err := osWriteFile(path, data, 0o644); err != nil {
		die(err)
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
}

func ghPRSymbolicStatus() (string, error) {
	branch, err := runGitOutput("", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not determine current branch: %w", err)
	}
	branch = strings.TrimSpace(branch)

	cmd := execCommand("gh", "pr", "view", branch, "--json", "state,isDraft")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isExecNotFound(err) {
			return "", errors.New("gh CLI is not installed")
		}
		outStr := string(out)
		if strings.Contains(outStr, "auth login") || strings.Contains(outStr, "not logged") {
			return "", errors.New("gh CLI is not configured (run gh auth login)")
		}
		if strings.Contains(outStr, "no pull requests found") || strings.Contains(outStr, "Could not resolve") {
			return "working", nil
		}
		return "", fmt.Errorf("gh pr view: %w\n%s", err, strings.TrimSpace(outStr))
	}

	var pr struct {
		State   string `json:"state"`
		IsDraft bool   `json:"isDraft"`
	}
	if err := json.Unmarshal(out, &pr); err != nil {
		return "", fmt.Errorf("gh: invalid JSON: %w", err)
	}

	switch {
	case pr.State == "MERGED":
		return "testing", nil
	case pr.State == "CLOSED":
		return "", nil
	case pr.IsDraft:
		return "", nil
	default:
		return "review", nil
	}
}

func isExecNotFound(err error) bool {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return errors.Is(execErr.Err, exec.ErrNotFound)
	}
	return false
}
