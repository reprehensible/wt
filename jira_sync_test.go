package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestJiraStatusSyncNoPR(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "To Do"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
	}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}
	transitioned := false
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		transitioned = true
		return nil, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			// Simulate "no pull requests found"
			return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix-bug\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd(nil)

	if !transitioned {
		t.Fatalf("expected transition to working")
	}
	if !strings.Contains(buf.String(), "PROJ-123 → In Progress") {
		t.Fatalf("expected transition message, got %q", buf.String())
	}
}

func TestJiraStatusSyncDraftPR(t *testing.T) {
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":true}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix-bug\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd(nil)

	// Draft PR → do nothing
	if buf.String() != "" {
		t.Fatalf("expected no output for draft PR, got %q", buf.String())
	}
}

func TestJiraStatusSyncOpenPR(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "In Progress"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "2", Name: "Review", To: jiraStatus{Name: "In Review"}},
	}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) { return nil, nil }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix-bug\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"review":"In Review"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd(nil)

	if !strings.Contains(buf.String(), "PROJ-123 → In Review") {
		t.Fatalf("expected transition to review, got %q", buf.String())
	}
}

func TestJiraStatusSyncMergedPR(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "In Review"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "3", Name: "Test", To: jiraStatus{Name: "In Testing"}},
	}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) { return nil, nil }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"MERGED","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix-bug\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"testing":"In Testing"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd(nil)

	if !strings.Contains(buf.String(), "PROJ-123 → In Testing") {
		t.Fatalf("expected transition to testing, got %q", buf.String())
	}
}

func TestJiraStatusSyncClosedPR(t *testing.T) {
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"CLOSED","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix-bug\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd(nil)

	if buf.String() != "" {
		t.Fatalf("expected no output for closed PR, got %q", buf.String())
	}
}

func TestJiraStatusSyncExplicitKey(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-999", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "To Do"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
	}}
	trBody, _ := json.Marshal(tr)

	var fetchedKey string
	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		if strings.Contains(url, "PROJ-999") {
			fetchedKey = "PROJ-999"
		}
		return issueBody, nil
	}
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) { return nil, nil }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			// Simulate "no pull requests found" → working
			return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-999-feature\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd([]string{"PROJ-999"})

	if fetchedKey != "PROJ-999" {
		t.Fatalf("expected PROJ-999, got %q", fetchedKey)
	}
	if !strings.Contains(buf.String(), "PROJ-999 → In Progress") {
		t.Fatalf("expected transition message, got %q", buf.String())
	}
}

func TestJiraStatusSyncNoConfig(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osGetenv = oldGetenv
		jiraGet = oldGet
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "To Do"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)

	jiraGet = func(url, user, token string) ([]byte, error) { return issueBody, nil }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "no jira status mappings configured") {
			t.Fatalf("expected config hint error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}

func TestJiraStatusSyncGhNotInstalled(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			// Return a command that won't be found
			return exec.Command("__nonexistent_binary_for_test__")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "gh CLI is not installed") {
			t.Fatalf("expected gh not installed error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}

func TestJiraStatusSyncGhNotConfigured(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return exec.Command("sh", "-c", "echo 'To get started with GitHub CLI, please run:  gh auth login' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "gh CLI is not configured") {
			t.Fatalf("expected gh not configured error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}

func TestJiraStatusSyncAlreadyAtStatus(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "In Review"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)

	jiraGet = func(url, user, token string) ([]byte, error) { return issueBody, nil }
	transitioned := false
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		transitioned = true
		return nil, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"review":"In Review"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd(nil)

	if transitioned {
		t.Fatalf("expected no transition when already at status")
	}
	if !strings.Contains(buf.String(), "PROJ-123: already In Review") {
		t.Fatalf("expected already message, got %q", buf.String())
	}
}

func TestJiraStatusSyncInferFail(t *testing.T) {
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

	jiraStatusSyncCmd(nil)
}

func TestJiraStatusSyncMissingEnv(t *testing.T) {
	oldGetenv := osGetenv
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	osGetenv = func(key string) string { return "" }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		return exec.Command("sh", "-c", "exit 1")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"review":"In Review"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "JIRA_URL") {
			t.Fatalf("expected JIRA env var error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}

// --- ghPRSymbolicStatus tests ---

func TestGhPRSymbolicStatus(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	t.Run("open non-draft", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		status, err := ghPRSymbolicStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "review" {
			t.Fatalf("expected review, got %q", status)
		}
	})

	t.Run("draft", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return cmdWithOutput(`{"state":"OPEN","isDraft":true}`)
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		status, err := ghPRSymbolicStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "" {
			t.Fatalf("expected empty for draft, got %q", status)
		}
	})

	t.Run("merged", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return cmdWithOutput(`{"state":"MERGED","isDraft":false}`)
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		status, err := ghPRSymbolicStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "testing" {
			t.Fatalf("expected testing, got %q", status)
		}
	})

	t.Run("closed", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return cmdWithOutput(`{"state":"CLOSED","isDraft":false}`)
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		status, err := ghPRSymbolicStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "" {
			t.Fatalf("expected empty for closed, got %q", status)
		}
	})

	t.Run("no PR", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		status, err := ghPRSymbolicStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "working" {
			t.Fatalf("expected working, got %q", status)
		}
	})

	t.Run("could not resolve", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return exec.Command("sh", "-c", "echo 'Could not resolve to a pull request' >&2; exit 1")
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		status, err := ghPRSymbolicStatus()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != "working" {
			t.Fatalf("expected working, got %q", status)
		}
	})

	t.Run("auth error", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return exec.Command("sh", "-c", "echo 'To get started with GitHub CLI, please run:  gh auth login' >&2; exit 1")
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		_, err := ghPRSymbolicStatus()
		if err == nil || !strings.Contains(err.Error(), "gh CLI is not configured") {
			t.Fatalf("expected gh not configured error, got %v", err)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return exec.Command("__nonexistent_binary_for_test__")
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		_, err := ghPRSymbolicStatus()
		if err == nil || !strings.Contains(err.Error(), "gh CLI is not installed") {
			t.Fatalf("expected gh not installed error, got %v", err)
		}
	})

	t.Run("other error", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return exec.Command("sh", "-c", "echo 'something went wrong' >&2; exit 1")
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		_, err := ghPRSymbolicStatus()
		if err == nil || !strings.Contains(err.Error(), "gh pr view") {
			t.Fatalf("expected gh pr view error, got %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			if name == "gh" {
				return cmdWithOutput(`not json`)
			}
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput("feature-branch\n")
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		_, err := ghPRSymbolicStatus()
		if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("expected invalid JSON error, got %v", err)
		}
	})

	t.Run("git error", func(t *testing.T) {
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		_, err := ghPRSymbolicStatus()
		if err == nil || !strings.Contains(err.Error(), "could not determine current branch") {
			t.Fatalf("expected branch error, got %v", err)
		}
	})
}

// --- isExecNotFound tests ---

func TestIsExecNotFound(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		err := &exec.Error{Name: "gh", Err: exec.ErrNotFound}
		if !isExecNotFound(err) {
			t.Fatalf("expected true for exec.ErrNotFound")
		}
	})

	t.Run("false - other error", func(t *testing.T) {
		err := errors.New("something else")
		if isExecNotFound(err) {
			t.Fatalf("expected false for non-exec error")
		}
	})

	t.Run("false - exec error not ErrNotFound", func(t *testing.T) {
		err := &exec.Error{Name: "gh", Err: errors.New("other")}
		if isExecNotFound(err) {
			t.Fatalf("expected false for non-ErrNotFound exec error")
		}
	})
}

// --- jira status sync routing test ---

func TestJiraStatusCmdRoutesToSync(t *testing.T) {
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
		// The sync cmd was invoked (fails because main branch has no issue key)
		if !strings.Contains(buf.String(), "does not contain an issue key") {
			t.Fatalf("expected branch error from sync, got %q", buf.String())
		}
	}()

	jiraStatusCmd([]string{"sync"})
}

func TestJiraStatusSyncGitBranchError(t *testing.T) {
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

	jiraStatusSyncCmd(nil)
}

func TestJiraStatusSyncConfigError(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{bad`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "invalid config") {
			t.Fatalf("expected config error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd([]string{"PROJ-123"})
}

func TestJiraStatusSyncFetchError(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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
		return nil, errors.New("fetch fail")
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		return exec.Command("sh", "-c", "exit 1")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"review":"In Review"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "fetch fail") {
			t.Fatalf("expected fetch error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}

func TestJiraStatusSyncSetStatusError(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "To Do"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{
		{ID: "1", Name: "Start", To: jiraStatus{Name: "In Progress"}},
	}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		return nil, errors.New("post fail")
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stderr = &buf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(buf.String(), "post fail") {
			t.Fatalf("expected post fail error, got %q", buf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}

func TestReverseSymbolic(t *testing.T) {
	t.Run("type override match", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "In Progress"},
			Types: map[string]map[string]string{
				"story": {"working": "In Development"},
			},
		}}}
		got := reverseSymbolic(cfg, "Story", "In Development")
		if got != "working" {
			t.Fatalf("expected working, got %q", got)
		}
	})

	t.Run("default match", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"review": "In Review"},
		}}}
		got := reverseSymbolic(cfg, "Story", "In Review")
		if got != "review" {
			t.Fatalf("expected review, got %q", got)
		}
	})

	t.Run("no match", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "In Progress"},
		}}}
		got := reverseSymbolic(cfg, "Story", "Unknown Status")
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "in progress"},
		}}}
		got := reverseSymbolic(cfg, "Story", "In Progress")
		if got != "working" {
			t.Fatalf("expected working, got %q", got)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		cfg := wtConfig{}
		got := reverseSymbolic(cfg, "Story", "In Progress")
		if got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})
}

func TestJiraStatusSyncDryRunNoPR(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "To Do"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)

	jiraGet = func(url, user, token string) ([]byte, error) { return issueBody, nil }
	posted := false
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		posted = true
		return nil, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix-bug\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd([]string{"--dry-run"})

	if posted {
		t.Fatalf("expected no jiraPost call with dry run")
	}
	if !strings.Contains(buf.String(), "To Do → In Progress (dry run)") {
		t.Fatalf("expected dry run message, got %q", buf.String())
	}
}

func TestJiraStatusSyncDryRunAlready(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "In Progress"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)

	jiraGet = func(url, user, token string) ([]byte, error) { return issueBody, nil }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd([]string{"-n"})

	if !strings.Contains(buf.String(), "already In Progress") {
		t.Fatalf("expected already message, got %q", buf.String())
	}
}

func TestJiraStatusSyncDryRunOpenPR(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "In Progress"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)

	jiraGet = func(url, user, token string) ([]byte, error) { return issueBody, nil }
	posted := false
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		posted = true
		return nil, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return cmdWithOutput(`{"state":"OPEN","isDraft":false}`)
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-123-fix\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"review":"In Review"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd([]string{"--dry-run"})

	if posted {
		t.Fatalf("expected no jiraPost call with dry run")
	}
	if !strings.Contains(buf.String(), "In Progress → In Review (dry run)") {
		t.Fatalf("expected dry run message, got %q", buf.String())
	}
}

func TestJiraStatusSyncDryRunExplicitKey(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldPost := jiraPost
	oldExec := execCommand
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		jiraPost = oldPost
		execCommand = oldExec
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
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

	issue := jiraIssue{Key: "PROJ-999", Fields: jiraFields{
		Summary:   "Test",
		Status:    jiraStatus{Name: "To Do"},
		IssueType: jiraIssueType{Name: "Story"},
	}}
	issueBody, _ := json.Marshal(issue)

	jiraGet = func(url, user, token string) ([]byte, error) { return issueBody, nil }
	posted := false
	jiraPost = func(url, user, token string, body []byte) ([]byte, error) {
		posted = true
		return nil, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return exec.Command("sh", "-c", "echo 'no pull requests found' >&2; exit 1")
		}
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-999-feature\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraStatusSyncCmd([]string{"-n", "PROJ-999"})

	if posted {
		t.Fatalf("expected no jiraPost call with dry run")
	}
	if !strings.Contains(buf.String(), "PROJ-999: To Do → In Progress (dry run)") {
		t.Fatalf("expected dry run message, got %q", buf.String())
	}
}

func TestJiraStatusSyncNoConfigDies(t *testing.T) {
	oldExec := execCommand
	oldExit := exitFunc
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		execCommand = oldExec
		exitFunc = oldExit
		stderr = oldErr
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
	}()

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return cmdWithOutput("PROJ-456-fix\n")
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }

	var errBuf bytes.Buffer
	stderr = &errBuf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "no jira status mappings configured") {
			t.Fatalf("expected config hint, got %q", errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "wt jira config --init") {
			t.Fatalf("expected --init hint, got %q", errBuf.String())
		}
	}()

	jiraStatusSyncCmd(nil)
}
