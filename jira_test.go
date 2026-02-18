package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"Fix login timeout", 50, "fix-login-timeout"},
		{"Hello, World! 123", 50, "hello-world-123"},
		{"  --leading trailing--  ", 50, "leading-trailing"},
		{"A very long title that should be truncated at word boundary here", 30, "a-very-long-title-that-should"},
		{"", 50, ""},
		{"---", 50, ""},
	}
	for _, tt := range tests {
		got := slugify(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("slugify(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestJiraBranchName(t *testing.T) {
	tests := []struct {
		key     string
		summary string
		want    string
	}{
		{"PROJ-123", "Fix login timeout", "PROJ-123-fix-login-timeout"},
		{"PROJ-456", "", "PROJ-456"},
		{"PROJ-789", "---", "PROJ-789"},
	}
	for _, tt := range tests {
		got := jiraBranchName(tt.key, tt.summary)
		if got != tt.want {
			t.Errorf("jiraBranchName(%q, %q) = %q, want %q", tt.key, tt.summary, got, tt.want)
		}
	}
}

func TestRenderIssueMD(t *testing.T) {
	// Full issue
	issue := jiraIssue{
		Key: "PROJ-123",
		Fields: jiraFields{
			Summary:     "Fix login timeout",
			Description: "Users see a timeout after 30s.",
			Comment: jiraComments{
				Comments: []jiraComment{
					{
						Author:  jiraAuthor{DisplayName: "Jane Smith"},
						Body:    "I can reproduce this.",
						Created: "2024-01-15T10:30:00.000+0000",
					},
				},
			},
		},
	}
	md := renderIssueMD(issue)
	if !strings.Contains(md, "# PROJ-123: Fix login timeout") {
		t.Fatalf("expected title in md: %s", md)
	}
	if !strings.Contains(md, "## Description") {
		t.Fatalf("expected description section: %s", md)
	}
	if !strings.Contains(md, "## Comments") {
		t.Fatalf("expected comments section: %s", md)
	}
	if !strings.Contains(md, "Jane Smith") {
		t.Fatalf("expected author in md: %s", md)
	}

	// No description, no comments
	issue2 := jiraIssue{
		Key:    "PROJ-456",
		Fields: jiraFields{Summary: "Simple bug"},
	}
	md2 := renderIssueMD(issue2)
	if strings.Contains(md2, "## Description") {
		t.Fatalf("expected no description section: %s", md2)
	}
	if strings.Contains(md2, "## Comments") {
		t.Fatalf("expected no comments section: %s", md2)
	}

	// Description only
	issue3 := jiraIssue{
		Key:    "PROJ-789",
		Fields: jiraFields{Summary: "With desc", Description: "Some desc"},
	}
	md3 := renderIssueMD(issue3)
	if !strings.Contains(md3, "## Description") {
		t.Fatalf("expected description: %s", md3)
	}
	if strings.Contains(md3, "## Comments") {
		t.Fatalf("expected no comments: %s", md3)
	}

	// Comments only
	issue4 := jiraIssue{
		Key: "PROJ-000",
		Fields: jiraFields{
			Summary: "With comments",
			Comment: jiraComments{
				Comments: []jiraComment{
					{Author: jiraAuthor{DisplayName: "Bob"}, Body: "test", Created: "2024-01-01T00:00:00.000+0000"},
				},
			},
		},
	}
	md4 := renderIssueMD(issue4)
	if strings.Contains(md4, "## Description") {
		t.Fatalf("expected no description: %s", md4)
	}
	if !strings.Contains(md4, "## Comments") {
		t.Fatalf("expected comments: %s", md4)
	}
}

func TestJiraGetDefaultSuccess(t *testing.T) {
	issue := jiraIssue{Key: "TEST-1", Fields: jiraFields{Summary: "Test"}}
	body, _ := json.Marshal(issue)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	got, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/TEST-1", "user", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("unexpected body: %s", string(got))
	}
}

func TestJiraGetDefault404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/NOPE-1", "user", "token")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 error, got %v", err)
	}
}

func TestJiraGetDefault401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/TEST-1", "bad", "creds")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestJiraGetDefaultUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := jiraGetDefault(srv.URL+"/rest/api/2/issue/TEST-1", "user", "token")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

func TestJiraGetDefaultNetworkError(t *testing.T) {
	_, err := jiraGetDefault("http://127.0.0.1:1/bad", "user", "token")
	if err == nil {
		t.Fatalf("expected network error")
	}
}

func TestJiraCmdSuccess(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	var writtenPath string
	var writtenData []byte
	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		writtenPath = name
		writtenData = data
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"new", "-S", "PROJ-123"})

	wtPath := worktreePath(repo, "PROJ-123-fix-login")
	if !strings.Contains(buf.String(), wtPath) {
		t.Fatalf("expected wtPath in output, got %q", buf.String())
	}
	if writtenPath != filepath.Join(wtPath, "PROJ-123.md") {
		t.Fatalf("expected md at %s, got %s", filepath.Join(wtPath, "PROJ-123.md"), writtenPath)
	}
	if !strings.Contains(string(writtenData), "# PROJ-123: Fix login") {
		t.Fatalf("expected issue content in md, got %s", string(writtenData))
	}
}

func TestJiraCmdBranchOverride(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"new", "-S", "-b", "my-branch", "PROJ-123"})

	wtPath := worktreePath(repo, "my-branch")
	if !strings.Contains(buf.String(), wtPath) {
		t.Fatalf("expected wtPath with custom branch in output, got %q", buf.String())
	}
}

func TestJiraCmdTmux(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldTmuxEnv := os.Getenv("TMUX")
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
		_ = os.Setenv("TMUX", oldTmuxEnv)
	}()

	_ = os.Unsetenv("TMUX")

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	tmuxCalled := false
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			tmuxCalled = true
			return exec.Command("sh", "-c", "exit 0")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"new", "-S", "-t", "PROJ-123"})

	if !tmuxCalled {
		t.Fatalf("expected tmux to be called")
	}
}

func TestJiraCmdMissingIssueKey(t *testing.T) {
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		exitFunc = oldExit
		stderr = oldErr
	}()

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "issue key required") {
			t.Fatalf("expected issue key error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"new"})
}

func TestJiraCmdMissingEnvVars(t *testing.T) {
	oldGetenv := osGetenv
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string { return "" }

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "JIRA_URL") {
			t.Fatalf("expected env var error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdAPIError(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	jiraGet = func(url, user, token string) ([]byte, error) {
		return nil, errors.New("jira: issue not found (404)")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "404") {
			t.Fatalf("expected 404 error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdInvalidJSON(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	jiraGet = func(url, user, token string) ([]byte, error) {
		return []byte("not json"), nil
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "invalid response") {
			t.Fatalf("expected invalid response error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdWriteError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return errors.New("write fail")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "write fail") {
			t.Fatalf("expected write error, got %q", buf.String())
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdRepoRootError(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdAddWorktreeError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdMainWorktreeError(t *testing.T) {
	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		// worktree list fails → gitMainWorktree error
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraCmd([]string{"new", "PROJ-123"})
}

func TestJiraCmdTmuxError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldExit := exitFunc
	oldErr := stderr
	oldTmuxEnv := os.Getenv("TMUX")
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		exitFunc = oldExit
		stderr = oldErr
		_ = os.Setenv("TMUX", oldTmuxEnv)
	}()

	_ = os.Unsetenv("TMUX")

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "tmux" {
			return exec.Command("sh", "-c", "exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraCmd([]string{"new", "-S", "-t", "PROJ-123"})
}

func TestJiraCmdTrailingSlashURL(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com/"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	var gotURL string
	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Test"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		gotURL = url
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"new", "-S", "PROJ-123"})

	if strings.Contains(gotURL, "//rest") {
		t.Fatalf("expected trailing slash stripped, got URL %q", gotURL)
	}
}

func TestMainHelpIncludesJira(t *testing.T) {
	oldErr := stderr
	defer func() { stderr = oldErr }()

	var buf bytes.Buffer
	stderr = &buf
	printUsage()

	if !strings.Contains(buf.String(), "jira") {
		t.Fatalf("expected jira in usage output, got %q", buf.String())
	}
}

func TestAddWorktreeSuccess(t *testing.T) {
	repo := t.TempDir()

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	wtPath, err := addWorktree(repo, repo, "test-branch", "", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := worktreePath(repo, "test-branch")
	if wtPath != expected {
		t.Fatalf("expected %q, got %q", expected, wtPath)
	}
}

func TestAddWorktreeEmptyBranch(t *testing.T) {
	_, err := addWorktree("/repo", "/repo", "", "", true, false)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestJiraCmdNoCopyConfig(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"new", "-S", "-C", "PROJ-123"})

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestJiraGetDefaultInvalidURL(t *testing.T) {
	_, err := jiraGetDefault("://bad\x7f", "user", "token")
	if err == nil {
		t.Fatalf("expected error for invalid URL")
	}
}

func TestJiraCmdNoCopyLibs(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return body, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput(repo)
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "list" {
			return cmdWithOutput(fmt.Sprintf("worktree %s\nbranch refs/heads/main\n", repo))
		}
		if len(args) >= 2 && args[0] == "show-ref" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraCmd([]string{"new", "-S", "-L", "PROJ-123"})

	if buf.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestJiraDispatcher(t *testing.T) {
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		exitFunc = oldExit
		stderr = oldErr
	}()

	// Verify "status" routes through dispatcher
	t.Run("status routes", func(t *testing.T) {
		oldGetenv := osGetenv
		oldGet := jiraGet
		oldOut := stdout
		defer func() {
			osGetenv = oldGetenv
			jiraGet = oldGet
			stdout = oldOut
		}()
		osGetenv = func(key string) string {
			switch key {
			case "JIRA_URL":
				return "https://jira.example.com"
			case "JIRA_USER":
				return "user"
			case "JIRA_TOKEN":
				return "token"
			}
			return ""
		}
		issue := jiraIssue{Key: "PROJ-1", Fields: jiraFields{Summary: "T", Status: jiraStatus{Name: "Open"}}}
		issueBody, _ := json.Marshal(issue)
		tr := jiraTransitionsResponse{}
		trBody, _ := json.Marshal(tr)
		jiraGet = func(url, user, token string) ([]byte, error) {
			if strings.Contains(url, "/transitions") {
				return trBody, nil
			}
			return issueBody, nil
		}
		var buf bytes.Buffer
		stdout = &buf
		jiraCmd([]string{"status", "PROJ-1"})
		if !strings.Contains(buf.String(), "PROJ-1: Open") {
			t.Fatalf("expected status output, got %q", buf.String())
		}
	})

	t.Run("config routes", func(t *testing.T) {
		oldReadFile := osReadFile
		oldHomeDir := osUserHomeDir
		oldOut := stdout
		defer func() {
			osReadFile = oldReadFile
			osUserHomeDir = oldHomeDir
			stdout = oldOut
		}()
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }
		// execCommand already stubbed to fail git → no repo config
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		var buf bytes.Buffer
		stdout = &buf
		jiraCmd([]string{"config"})
		if !strings.Contains(buf.String(), "no config found") {
			t.Fatalf("expected no config found, got %q", buf.String())
		}
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"empty", nil, "usage: wt jira"},
		{"unknown", []string{"bogus"}, "unknown jira command: bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			stderr = &buf
			exitFunc = func(code int) { panic(code) }

			defer func() {
				if r := recover(); r != 1 {
					t.Fatalf("expected exit 1, got %v", r)
				}
				if !strings.Contains(buf.String(), tt.want) {
					t.Fatalf("expected %q in output, got %q", tt.want, buf.String())
				}
			}()

			jiraCmd(tt.args)
		})
	}
}

func TestJiraIssueKeyFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"PROJ-123-fix-login-timeout", "PROJ-123"},
		{"PROJ-123", "PROJ-123"},
		{"AB-1", "AB-1"},
		{"main", ""},
		{"feature/something", ""},
		{"", ""},
		{"lowercase-123", ""},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := jiraIssueKeyFromBranch(tt.branch)
			if got != tt.want {
				t.Fatalf("jiraIssueKeyFromBranch(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

func TestJiraEnv(t *testing.T) {
	oldGetenv := osGetenv
	defer func() { osGetenv = oldGetenv }()

	t.Run("success", func(t *testing.T) {
		osGetenv = func(key string) string {
			switch key {
			case "JIRA_URL":
				return "https://jira.example.com/"
			case "JIRA_USER":
				return "user"
			case "JIRA_TOKEN":
				return "token"
			}
			return ""
		}
		url, user, token, err := jiraEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url != "https://jira.example.com" {
			t.Fatalf("expected trailing slash stripped, got %q", url)
		}
		if user != "user" || token != "token" {
			t.Fatalf("unexpected user/token: %q %q", user, token)
		}
	})

	t.Run("missing", func(t *testing.T) {
		osGetenv = func(key string) string { return "" }
		_, _, _, err := jiraEnv()
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "JIRA_URL") {
			t.Fatalf("expected JIRA_URL in error, got %q", err.Error())
		}
	})
}

func TestJiraFetchIssue(t *testing.T) {
	oldGet := jiraGet
	defer func() { jiraGet = oldGet }()

	t.Run("success", func(t *testing.T) {
		issue := jiraIssue{Key: "PROJ-1", Fields: jiraFields{
			Summary:   "Test",
			Status:    jiraStatus{Name: "Open"},
			IssueType: jiraIssueType{Name: "Story"},
		}}
		body, _ := json.Marshal(issue)
		jiraGet = func(url, user, token string) ([]byte, error) {
			if !strings.Contains(url, "fields=summary,description,comment,status,issuetype") {
				t.Fatalf("expected issuetype in fields, got %q", url)
			}
			return body, nil
		}
		got, err := jiraFetchIssue("https://jira.example.com", "PROJ-1", "user", "token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Key != "PROJ-1" {
			t.Fatalf("expected PROJ-1, got %q", got.Key)
		}
		if got.Fields.Status.Name != "Open" {
			t.Fatalf("expected status Open, got %q", got.Fields.Status.Name)
		}
		if got.Fields.IssueType.Name != "Story" {
			t.Fatalf("expected issue type Story, got %q", got.Fields.IssueType.Name)
		}
	})

	t.Run("api error", func(t *testing.T) {
		jiraGet = func(url, user, token string) ([]byte, error) {
			return nil, errors.New("network fail")
		}
		_, err := jiraFetchIssue("https://jira.example.com", "PROJ-1", "user", "token")
		if err == nil || !strings.Contains(err.Error(), "network fail") {
			t.Fatalf("expected network fail error, got %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		jiraGet = func(url, user, token string) ([]byte, error) {
			return []byte("not json"), nil
		}
		_, err := jiraFetchIssue("https://jira.example.com", "PROJ-1", "user", "token")
		if err == nil || !strings.Contains(err.Error(), "invalid response") {
			t.Fatalf("expected invalid response error, got %v", err)
		}
	})
}

func TestJiraSetStatus(t *testing.T) {
	oldGet := jiraGet
	oldPost := jiraPost
	defer func() {
		jiraGet = oldGet
		jiraPost = oldPost
	}()

	t.Run("success", func(t *testing.T) {
		tr := jiraTransitionsResponse{Transitions: []jiraTransition{
			{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
			{ID: "2", Name: "Done", To: jiraStatus{Name: "Done"}},
		}}
		trBody, _ := json.Marshal(tr)
		jiraGet = func(url, user, token string) ([]byte, error) {
			return trBody, nil
		}
		var postURL string
		var postBody []byte
		jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
			postURL = url
			postBody = body
			return nil, nil
		}
		err := jiraSetStatus("https://jira.example.com", "PROJ-1", "In Progress", "user", "token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(postURL, "/transitions") {
			t.Fatalf("expected transitions URL, got %q", postURL)
		}
		if !strings.Contains(string(postBody), `"id":"1"`) {
			t.Fatalf("expected transition id 1, got %q", string(postBody))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		tr := jiraTransitionsResponse{Transitions: []jiraTransition{
			{ID: "5", Name: "Start", To: jiraStatus{Name: "In Progress"}},
		}}
		trBody, _ := json.Marshal(tr)
		jiraGet = func(url, user, token string) ([]byte, error) {
			return trBody, nil
		}
		jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
			return nil, nil
		}
		err := jiraSetStatus("https://jira.example.com", "PROJ-1", "in progress", "user", "token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		tr := jiraTransitionsResponse{Transitions: []jiraTransition{
			{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
		}}
		trBody, _ := json.Marshal(tr)
		jiraGet = func(url, user, token string) ([]byte, error) {
			return trBody, nil
		}
		err := jiraSetStatus("https://jira.example.com", "PROJ-1", "Nonexistent", "user", "token")
		if err == nil || !strings.Contains(err.Error(), "no transition") {
			t.Fatalf("expected no transition error, got %v", err)
		}
	})

	t.Run("get error", func(t *testing.T) {
		jiraGet = func(url, user, token string) ([]byte, error) {
			return nil, errors.New("get fail")
		}
		err := jiraSetStatus("https://jira.example.com", "PROJ-1", "Done", "user", "token")
		if err == nil || !strings.Contains(err.Error(), "get fail") {
			t.Fatalf("expected get fail error, got %v", err)
		}
	})

	t.Run("invalid transitions json", func(t *testing.T) {
		jiraGet = func(url, user, token string) ([]byte, error) {
			return []byte("bad"), nil
		}
		err := jiraSetStatus("https://jira.example.com", "PROJ-1", "Done", "user", "token")
		if err == nil || !strings.Contains(err.Error(), "invalid transitions") {
			t.Fatalf("expected invalid transitions error, got %v", err)
		}
	})

	t.Run("post error", func(t *testing.T) {
		tr := jiraTransitionsResponse{Transitions: []jiraTransition{
			{ID: "1", Name: "Start", To: jiraStatus{Name: "Done"}},
		}}
		trBody, _ := json.Marshal(tr)
		jiraGet = func(url, user, token string) ([]byte, error) {
			return trBody, nil
		}
		jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
			return nil, errors.New("post fail")
		}
		err := jiraSetStatus("https://jira.example.com", "PROJ-1", "Done", "user", "token")
		if err == nil || !strings.Contains(err.Error(), "post fail") {
			t.Fatalf("expected post fail error, got %v", err)
		}
	})
}

func TestJiraStatusCmdShow(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
		execCommand = oldExec
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Test", Status: jiraStatus{Name: "Open"}, IssueType: jiraIssueType{Name: "Story"}}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
		{ID: "2", Name: "Close", To: jiraStatus{Name: "Done"}},
	}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress","done":"Done"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusCmd([]string{"PROJ-123"})

	out := buf.String()
	if !strings.Contains(out, "PROJ-123: Open") {
		t.Fatalf("expected status line, got %q", out)
	}
	if !strings.Contains(out, "In Progress (working)") {
		t.Fatalf("expected annotated In Progress transition, got %q", out)
	}
	if !strings.Contains(out, "Done (done)") {
		t.Fatalf("expected annotated Done transition, got %q", out)
	}
}

func TestJiraStatusCmdSet(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
	}}
	trBody, _ := json.Marshal(tr)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return trBody, nil
	}
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		return nil, nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusCmd([]string{"PROJ-123", "In Progress"})

	if !strings.Contains(buf.String(), "PROJ-123 → In Progress") {
		t.Fatalf("expected transition message, got %q", buf.String())
	}
}

func TestJiraStatusCmdNotFound(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
	}}
	trBody, _ := json.Marshal(tr)
	jiraGet = func(url, user, token string) ([]byte, error) {
		return trBody, nil
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "no transition") {
			t.Fatalf("expected no transition error, got %q", buf.String())
		}
	}()

	jiraStatusCmd([]string{"PROJ-123", "Nonexistent"})
}

func TestJiraStatusCmdInferFromBranch(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExec := execCommand
	oldOut := stdout
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		execCommand = oldExec
		stdout = oldOut
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-456-fix-bug\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	issue := jiraIssue{Key: "PROJ-456", Fields: jiraFields{Summary: "Fix bug", Status: jiraStatus{Name: "To Do"}}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		if !strings.Contains(url, "PROJ-456") {
			t.Fatalf("expected PROJ-456 in URL, got %q", url)
		}
		return issueBody, nil
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusCmd(nil)

	if !strings.Contains(buf.String(), "PROJ-456: To Do") {
		t.Fatalf("expected status line, got %q", buf.String())
	}
}

func TestJiraStatusCmdInferFail(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("main\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "does not contain an issue key") {
			t.Fatalf("expected branch error, got %q", buf.String())
		}
	}()

	jiraStatusCmd(nil)
}

func TestJiraStatusCmdMissingEnv(t *testing.T) {
	oldGetenv := osGetenv
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string { return "" }

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "JIRA_URL") {
			t.Fatalf("expected env var error, got %q", buf.String())
		}
	}()

	jiraStatusCmd([]string{"PROJ-123"})
}

func TestJiraStatusCmdAPIError(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	jiraGet = func(url, user, token string) ([]byte, error) {
		return nil, errors.New("api fail")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "api fail") {
			t.Fatalf("expected api error, got %q", buf.String())
		}
	}()

	jiraStatusCmd([]string{"PROJ-123"})
}

func TestJiraStatusCmdTransitionsAPIError(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Test", Status: jiraStatus{Name: "Open"}}}
	issueBody, _ := json.Marshal(issue)
	calls := 0
	jiraGet = func(url, user, token string) ([]byte, error) {
		calls++
		if calls == 1 {
			return issueBody, nil
		}
		return nil, errors.New("transitions fail")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "transitions fail") {
			t.Fatalf("expected transitions error, got %q", buf.String())
		}
	}()

	jiraStatusCmd([]string{"PROJ-123"})
}

func TestJiraStatusCmdInferGitError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "could not determine current branch") {
			t.Fatalf("expected branch error, got %q", buf.String())
		}
	}()

	jiraStatusCmd(nil)
}

func TestJiraPostDefaultSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected json content type, got %q", r.Header.Get("Content-Type"))
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "token" {
			t.Fatalf("expected basic auth user/token, got %q/%q", user, pass)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, err := jiraPostDefault(srv.URL, "user", "token", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJiraPostDefaultError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := jiraPostDefault(srv.URL, "user", "token", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected error for 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 in error, got %q", err.Error())
	}
}

func TestJiraPostDefaultNetworkError(t *testing.T) {
	_, err := jiraPostDefault("http://127.0.0.1:1/bad", "user", "token", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected network error")
	}
}

func TestJiraPostDefaultInvalidURL(t *testing.T) {
	_, err := jiraPostDefault("://bad\x7f", "user", "token", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected error for invalid URL")
	}
}

func TestJiraStatusCmdTransitionsInvalidJSON(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osGetenv = func(key string) string {
		switch key {
		case "JIRA_URL":
			return "https://jira.example.com"
		case "JIRA_USER":
			return "user"
		case "JIRA_TOKEN":
			return "token"
		}
		return ""
	}

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Test", Status: jiraStatus{Name: "Open"}}}
	issueBody, _ := json.Marshal(issue)
	calls := 0
	jiraGet = func(url, user, token string) ([]byte, error) {
		calls++
		if calls == 1 {
			return issueBody, nil
		}
		return []byte("bad json"), nil
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "invalid transitions") {
			t.Fatalf("expected invalid transitions error, got %q", buf.String())
		}
	}()

	jiraStatusCmd([]string{"PROJ-123"})
}

// --- Config tests ---
