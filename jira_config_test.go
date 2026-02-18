package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	defer func() {
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
		execCommand = oldExec
	}()

	t.Run("global only", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
			}
			return nil, os.ErrNotExist
		}
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Jira.Status.Default["working"] != "In Progress" {
			t.Fatalf("expected In Progress, got %q", cfg.Jira.Status.Default["working"])
		}
	})

	t.Run("repo only", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		repo := t.TempDir()
		execCommand = func(name string, args ...string) *exec.Cmd {
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput(repo)
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join(repo, ".wt.json") {
				return []byte(`{"jira":{"status":{"default":{"review":"Code Review"}}}}`), nil
			}
			return nil, os.ErrNotExist
		}
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Jira.Status.Default["review"] != "Code Review" {
			t.Fatalf("expected Code Review, got %q", cfg.Jira.Status.Default["review"])
		}
	})

	t.Run("merge", func(t *testing.T) {
		repo := t.TempDir()
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput(repo)
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return []byte(`{"jira":{"status":{"default":{"working":"In Progress","review":"In Review"}}}}`), nil
			}
			if name == filepath.Join(repo, ".wt.json") {
				return []byte(`{"jira":{"status":{"default":{"review":"Code Review"}}}}`), nil
			}
			return nil, os.ErrNotExist
		}
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Jira.Status.Default["working"] != "In Progress" {
			t.Fatalf("expected In Progress, got %q", cfg.Jira.Status.Default["working"])
		}
		if cfg.Jira.Status.Default["review"] != "Code Review" {
			t.Fatalf("expected Code Review (repo override), got %q", cfg.Jira.Status.Default["review"])
		}
	})

	t.Run("neither exists", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Jira.Status.Default != nil {
			t.Fatalf("expected nil default, got %v", cfg.Jira.Status.Default)
		}
	})

	t.Run("invalid global JSON", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return []byte(`{bad`), nil
			}
			return nil, os.ErrNotExist
		}
		_, err := loadConfig()
		if err == nil || !strings.Contains(err.Error(), "invalid config") {
			t.Fatalf("expected invalid config error, got %v", err)
		}
	})

	t.Run("invalid repo JSON", func(t *testing.T) {
		repo := t.TempDir()
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput(repo)
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join(repo, ".wt.json") {
				return []byte(`not json`), nil
			}
			return nil, os.ErrNotExist
		}
		_, err := loadConfig()
		if err == nil || !strings.Contains(err.Error(), "invalid config") {
			t.Fatalf("expected invalid config error, got %v", err)
		}
	})

	t.Run("home dir error", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "", errors.New("no home") }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Jira.Status.Default != nil {
			t.Fatalf("expected nil default, got %v", cfg.Jira.Status.Default)
		}
	})

	t.Run("git root error", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return []byte(`{"jira":{"status":{"default":{"working":"WIP"}}}}`), nil
			}
			return nil, os.ErrNotExist
		}
		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Jira.Status.Default["working"] != "WIP" {
			t.Fatalf("expected WIP, got %q", cfg.Jira.Status.Default["working"])
		}
	})

	t.Run("global read error non-ENOENT", func(t *testing.T) {
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return nil, errors.New("permission denied")
			}
			return nil, os.ErrNotExist
		}
		_, err := loadConfig()
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("expected permission denied error, got %v", err)
		}
	})

	t.Run("repo read error non-ENOENT", func(t *testing.T) {
		repo := t.TempDir()
		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			if len(args) > 0 && args[0] == "-C" {
				args = args[2:]
			}
			if len(args) >= 2 && args[0] == "rev-parse" {
				return cmdWithOutput(repo)
			}
			return exec.Command("sh", "-c", "exit 0")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == filepath.Join(repo, ".wt.json") {
				return nil, errors.New("disk error")
			}
			return nil, os.ErrNotExist
		}
		_, err := loadConfig()
		if err == nil || !strings.Contains(err.Error(), "disk error") {
			t.Fatalf("expected disk error, got %v", err)
		}
	})
}

func TestMergeConfig(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := mergeConfig(wtConfig{}, wtConfig{})
		if result.Jira.Status.Default == nil {
			t.Fatalf("expected non-nil default map")
		}
	})

	t.Run("default override", func(t *testing.T) {
		global := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "In Progress", "review": "In Review"},
		}}}
		repo := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"review": "Code Review"},
		}}}
		result := mergeConfig(global, repo)
		if result.Jira.Status.Default["working"] != "In Progress" {
			t.Fatalf("expected In Progress, got %q", result.Jira.Status.Default["working"])
		}
		if result.Jira.Status.Default["review"] != "Code Review" {
			t.Fatalf("expected Code Review, got %q", result.Jira.Status.Default["review"])
		}
	})

	t.Run("types override", func(t *testing.T) {
		global := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Types: map[string]map[string]string{
				"story": {"working": "In Development", "review": "In Review"},
			},
		}}}
		repo := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Types: map[string]map[string]string{
				"story": {"review": "Code Review"},
			},
		}}}
		result := mergeConfig(global, repo)
		if result.Jira.Status.Types["story"]["working"] != "In Development" {
			t.Fatalf("expected In Development, got %q", result.Jira.Status.Types["story"]["working"])
		}
		if result.Jira.Status.Types["story"]["review"] != "Code Review" {
			t.Fatalf("expected Code Review, got %q", result.Jira.Status.Types["story"]["review"])
		}
	})

	t.Run("new type", func(t *testing.T) {
		global := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Types: map[string]map[string]string{
				"story": {"working": "In Dev"},
			},
		}}}
		repo := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Types: map[string]map[string]string{
				"bug": {"working": "Fixing"},
			},
		}}}
		result := mergeConfig(global, repo)
		if result.Jira.Status.Types["story"]["working"] != "In Dev" {
			t.Fatalf("expected In Dev, got %q", result.Jira.Status.Types["story"]["working"])
		}
		if result.Jira.Status.Types["bug"]["working"] != "Fixing" {
			t.Fatalf("expected Fixing, got %q", result.Jira.Status.Types["bug"]["working"])
		}
	})
}

func TestResolveStatus(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "In Progress"},
		}}}
		got, err := resolveStatus(cfg, "Story", "working")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "In Progress" {
			t.Fatalf("expected In Progress, got %q", got)
		}
	})

	t.Run("type override", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "In Progress"},
			Types: map[string]map[string]string{
				"story": {"working": "In Development"},
			},
		}}}
		got, err := resolveStatus(cfg, "Story", "working")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "In Development" {
			t.Fatalf("expected In Development, got %q", got)
		}
	})

	t.Run("fallthrough to default", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"review": "In Review"},
			Types: map[string]map[string]string{
				"story": {"working": "In Development"},
			},
		}}}
		got, err := resolveStatus(cfg, "Story", "review")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "In Review" {
			t.Fatalf("expected In Review, got %q", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Default: map[string]string{"working": "In Progress"},
		}}}
		_, err := resolveStatus(cfg, "Story", "unknown")
		if err == nil || !strings.Contains(err.Error(), "no status mapping") {
			t.Fatalf("expected no status mapping error, got %v", err)
		}
	})

	t.Run("case insensitive type", func(t *testing.T) {
		cfg := wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
			Types: map[string]map[string]string{
				"dev task": {"working": "Developing"},
			},
		}}}
		got, err := resolveStatus(cfg, "Dev Task", "working")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Developing" {
			t.Fatalf("expected Developing, got %q", got)
		}
	})

	t.Run("empty config", func(t *testing.T) {
		cfg := wtConfig{}
		_, err := resolveStatus(cfg, "Story", "working")
		if err == nil || !strings.Contains(err.Error(), "no status mapping") {
			t.Fatalf("expected no status mapping error, got %v", err)
		}
	})
}

// --- jiraNewCmd auto-transition tests ---

func TestJiraNewCmdAutoTransition(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldJiraPost := jiraPost
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		jiraPost = oldJiraPost
		execCommand = oldExec
		osWriteFile = oldWriteFile
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
		Summary:   "Fix login",
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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraNewCmd([]string{"PROJ-123"})

	if !transitioned {
		t.Fatalf("expected auto-transition to happen")
	}
	if !strings.Contains(buf.String(), "PROJ-123 → In Progress") {
		t.Fatalf("expected transition message, got %q", buf.String())
	}
}

func TestJiraNewCmdAutoTransitionNoConfig(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldJiraPost := jiraPost
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldErr := stderr
	oldExit := exitFunc
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		jiraPost = oldJiraPost
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
		stderr = oldErr
		exitFunc = oldExit
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) { return body, nil }

	transitioned := false
	jiraPost = func(url, user, token string, b []byte) ([]byte, error) {
		transitioned = true
		return nil, nil
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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if transitioned {
			t.Fatalf("expected no transition with no config")
		}
		// Worktree should still be created before die
		if !strings.Contains(buf.String(), repo+"-worktrees") {
			t.Fatalf("expected worktree path in output, got %q", buf.String())
		}
		if !strings.Contains(errBuf.String(), "no jira status mappings configured") {
			t.Fatalf("expected config hint, got %q", errBuf.String())
		}
	}()

	jiraNewCmd([]string{"PROJ-123"})
}

func TestJiraNewCmdAutoTransitionSkipFlag(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldJiraPost := jiraPost
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		jiraPost = oldJiraPost
		execCommand = oldExec
		osWriteFile = oldWriteFile
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
		Summary:   "Fix login",
		IssueType: jiraIssueType{Name: "Story"},
	}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) { return body, nil }

	transitioned := false
	jiraPost = func(url, user, token string, b []byte) ([]byte, error) {
		transitioned = true
		return nil, nil
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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraNewCmd([]string{"-S", "PROJ-123"})

	if transitioned {
		t.Fatalf("expected no transition with -S flag")
	}
}

func TestJiraNewCmdAutoTransitionNoMapping(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldJiraPost := jiraPost
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		jiraPost = oldJiraPost
		execCommand = oldExec
		osWriteFile = oldWriteFile
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
		Summary:   "Fix login",
		IssueType: jiraIssueType{Name: "Story"},
	}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) { return body, nil }

	transitioned := false
	jiraPost = func(url, user, token string, b []byte) ([]byte, error) {
		transitioned = true
		return nil, nil
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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	// Config exists but no "working" key
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"review":"In Review"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraNewCmd([]string{"PROJ-123"})

	if transitioned {
		t.Fatalf("expected no transition with no working mapping")
	}
	// Worktree should still be created
	if !strings.Contains(buf.String(), repo+"-worktrees") {
		t.Fatalf("expected worktree path, got %q", buf.String())
	}
}

func TestJiraNewCmdAutoTransitionAPIError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldJiraPost := jiraPost
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldErrOut := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		jiraPost = oldJiraPost
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
		stderr = oldErrOut
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
		Summary:   "Fix login",
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
		return nil, errors.New("transition api fail")
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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var outBuf bytes.Buffer
	stdout = &outBuf
	var errBuf bytes.Buffer
	stderr = &errBuf

	jiraNewCmd([]string{"PROJ-123"})

	// Worktree still created
	if !strings.Contains(outBuf.String(), repo+"-worktrees") {
		t.Fatalf("expected worktree path, got %q", outBuf.String())
	}
	// Warning on stderr
	if !strings.Contains(errBuf.String(), "transition api fail") {
		t.Fatalf("expected warning about transition, got %q", errBuf.String())
	}
}

func TestJiraNewCmdAutoTransitionConfigError(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldJiraPost := jiraPost
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldErrOut := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		jiraPost = oldJiraPost
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
		stderr = oldErrOut
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
		Summary:   "Fix login",
		IssueType: jiraIssueType{Name: "Story"},
	}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) { return body, nil }

	transitioned := false
	jiraPost = func(url, user, token string, b []byte) ([]byte, error) {
		transitioned = true
		return nil, nil
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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	// Invalid JSON config triggers config error warning
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{bad`), nil
		}
		return nil, os.ErrNotExist
	}

	var outBuf bytes.Buffer
	stdout = &outBuf
	var errBuf bytes.Buffer
	stderr = &errBuf

	jiraNewCmd([]string{"PROJ-123"})

	if transitioned {
		t.Fatalf("expected no transition when config is invalid")
	}
	if !strings.Contains(errBuf.String(), "warning: config:") {
		t.Fatalf("expected config warning on stderr, got %q", errBuf.String())
	}
	// Worktree still created
	if !strings.Contains(outBuf.String(), repo+"-worktrees") {
		t.Fatalf("expected worktree path, got %q", outBuf.String())
	}
}

// --- jiraStatusSyncCmd tests ---

func TestJiraConfigCmd(t *testing.T) {
	t.Run("defaults and types", func(t *testing.T) {
		oldOut := stdout
		oldReadFile := osReadFile
		oldHomeDir := osUserHomeDir
		oldExec := execCommand
		defer func() {
			stdout = oldOut
			osReadFile = oldReadFile
			osUserHomeDir = oldHomeDir
			execCommand = oldExec
		}()

		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return []byte(`{"jira":{"status":{"default":{"review":"In Review","working":"In Progress"},"types":{"story":{"working":"In Development"}}}}}`), nil
			}
			return nil, os.ErrNotExist
		}

		var buf bytes.Buffer
		stdout = &buf

		jiraConfigCmd(nil)

		out := buf.String()
		if !strings.Contains(out, "default:") {
			t.Fatalf("expected default section, got %q", out)
		}
		if !strings.Contains(out, "review → In Review") {
			t.Fatalf("expected review mapping, got %q", out)
		}
		if !strings.Contains(out, "working → In Progress") {
			t.Fatalf("expected working mapping, got %q", out)
		}
		if !strings.Contains(out, "story:") {
			t.Fatalf("expected story section, got %q", out)
		}
		if !strings.Contains(out, "working → In Development") {
			t.Fatalf("expected story override, got %q", out)
		}
	})

	t.Run("no config", func(t *testing.T) {
		oldOut := stdout
		oldReadFile := osReadFile
		oldHomeDir := osUserHomeDir
		oldExec := execCommand
		defer func() {
			stdout = oldOut
			osReadFile = oldReadFile
			osUserHomeDir = oldHomeDir
			execCommand = oldExec
		}()

		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }

		var buf bytes.Buffer
		stdout = &buf

		jiraConfigCmd(nil)

		if !strings.Contains(buf.String(), "no config found") {
			t.Fatalf("expected no config found, got %q", buf.String())
		}
	})

	t.Run("config error", func(t *testing.T) {
		oldExit := exitFunc
		oldErr := stderr
		oldReadFile := osReadFile
		oldHomeDir := osUserHomeDir
		oldExec := execCommand
		defer func() {
			exitFunc = oldExit
			stderr = oldErr
			osReadFile = oldReadFile
			osUserHomeDir = oldHomeDir
			execCommand = oldExec
		}()

		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
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
				t.Fatalf("expected invalid config error, got %q", buf.String())
			}
		}()

		jiraConfigCmd(nil)
	})

	t.Run("with issue key", func(t *testing.T) {
		oldOut := stdout
		oldReadFile := osReadFile
		oldHomeDir := osUserHomeDir
		oldExec := execCommand
		oldGetenv := osGetenv
		oldGet := jiraGet
		defer func() {
			stdout = oldOut
			osReadFile = oldReadFile
			osUserHomeDir = oldHomeDir
			execCommand = oldExec
			osGetenv = oldGetenv
			jiraGet = oldGet
		}()

		osUserHomeDir = func() (string, error) { return "/home/test", nil }
		execCommand = func(name string, args ...string) *exec.Cmd {
			return exec.Command("sh", "-c", "exit 1")
		}
		osReadFile = func(name string) ([]byte, error) {
			if name == "/home/test/.config/wt/config.json" {
				return []byte(`{"jira":{"status":{"default":{"working":"In Progress","review":"In Review"},"types":{"story":{"working":"In Development"}}}}}`), nil
			}
			return nil, os.ErrNotExist
		}
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

		var buf bytes.Buffer
		stdout = &buf

		jiraConfigCmd([]string{"PROJ-123"})

		out := buf.String()
		if !strings.Contains(out, "resolved (story):") {
			t.Fatalf("expected resolved section, got %q", out)
		}
		if !strings.Contains(out, "working → In Development") {
			t.Fatalf("expected resolved working override, got %q", out)
		}
		if !strings.Contains(out, "review → In Review") {
			t.Fatalf("expected resolved review fallthrough, got %q", out)
		}
	})
}

func TestJiraConfigCmdMissingEnv(t *testing.T) {
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	oldGetenv := osGetenv
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
		execCommand = oldExec
		osGetenv = oldGetenv
		exitFunc = oldExit
		stderr = oldErr
	}()

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}
	osGetenv = func(key string) string { return "" }

	var outBuf bytes.Buffer
	stdout = &outBuf
	var errBuf bytes.Buffer
	stderr = &errBuf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "JIRA_URL") {
			t.Fatalf("expected env var error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"PROJ-123"})
}

func TestJiraConfigCmdFetchError(t *testing.T) {
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
		execCommand = oldExec
		osGetenv = oldGetenv
		jiraGet = oldGet
		exitFunc = oldExit
		stderr = oldErr
	}()

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"}}}}`), nil
		}
		return nil, os.ErrNotExist
	}
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

	var outBuf bytes.Buffer
	stdout = &outBuf
	var errBuf bytes.Buffer
	stderr = &errBuf
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "fetch fail") {
			t.Fatalf("expected fetch error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"PROJ-123"})
}

func TestJiraConfigCmdEmptyTypeMap(t *testing.T) {
	oldOut := stdout
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	defer func() {
		stdout = oldOut
		osReadFile = oldReadFile
		osUserHomeDir = oldHomeDir
		execCommand = oldExec
	}()

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}
	osReadFile = func(name string) ([]byte, error) {
		if name == "/home/test/.config/wt/config.json" {
			return []byte(`{"jira":{"status":{"default":{"working":"In Progress"},"types":{"story":{}}}}}`), nil
		}
		return nil, os.ErrNotExist
	}

	var buf bytes.Buffer
	stdout = &buf

	jiraConfigCmd(nil)

	out := buf.String()
	if !strings.Contains(out, "default:") {
		t.Fatalf("expected default section, got %q", out)
	}
	// Empty type map should be skipped
	if strings.Contains(out, "story:") {
		t.Fatalf("expected empty story type to be skipped, got %q", out)
	}
}

func TestJiraStatusCmdShowNoConfig(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldOut := stdout
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		stdout = oldOut
		stderr = oldErr
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
	}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf

	jiraStatusCmd([]string{"PROJ-123"})

	out := buf.String()
	if !strings.Contains(out, "PROJ-123: Open") {
		t.Fatalf("expected status line, got %q", out)
	}
	// Without config, transitions should show without annotation
	if strings.Contains(out, "(working)") {
		t.Fatalf("expected no annotation without config, got %q", out)
	}
	if !strings.Contains(out, "  In Progress\n") {
		t.Fatalf("expected plain In Progress transition, got %q", out)
	}
	if !strings.Contains(errBuf.String(), "hint: run 'wt jira config --init'") {
		t.Fatalf("expected hint on stderr, got %q", errBuf.String())
	}
}

func TestMainHelpIncludesConfig(t *testing.T) {
	oldErr := stderr
	defer func() { stderr = oldErr }()

	var buf bytes.Buffer
	stderr = &buf
	printUsage()

	if !strings.Contains(buf.String(), "jira config") {
		t.Fatalf("expected jira config in usage output, got %q", buf.String())
	}

	// Detailed flags are in per-command help, not top-level
	buf.Reset()
	printJiraConfigUsage()
	if !strings.Contains(buf.String(), "--init") {
		t.Fatalf("expected --init in jira config help, got %q", buf.String())
	}

	buf.Reset()
	printJiraStatusUsage()
	if !strings.Contains(buf.String(), "--dry-run") {
		t.Fatalf("expected --dry-run in jira status help, got %q", buf.String())
	}
}

func TestHasStatusConfig(t *testing.T) {
	if hasStatusConfig(wtConfig{}) {
		t.Fatal("expected false for empty config")
	}
	if !hasStatusConfig(wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
		Default: map[string]string{"working": "In Progress"},
	}}}) {
		t.Fatal("expected true for defaults-only config")
	}
	if !hasStatusConfig(wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
		Types: map[string]map[string]string{"bug": {"working": "In Progress"}},
	}}}) {
		t.Fatal("expected true for types-only config")
	}
}

func TestTemplateConfig(t *testing.T) {
	cfg := templateConfig()
	if len(cfg.Jira.Status.Default) == 0 {
		t.Fatal("expected non-empty defaults")
	}
	for _, key := range []string{"working", "review", "testing", "done"} {
		if _, ok := cfg.Jira.Status.Default[key]; !ok {
			t.Fatalf("expected key %q in defaults", key)
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTrip wtConfig
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(roundTrip.Jira.Status.Default) != len(cfg.Jira.Status.Default) {
		t.Fatal("round-trip mismatch")
	}
}

func TestJiraConfigCmdInitGlobal(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	oldHomeDir := osUserHomeDir
	oldMkdir := osMkdirAll
	oldWriteFile := osWriteFile
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
		osUserHomeDir = oldHomeDir
		osMkdirAll = oldMkdir
		osWriteFile = oldWriteFile
	}()

	var buf bytes.Buffer
	stdout = &buf
	stdin = strings.NewReader("g\n")
	exitFunc = func(code int) { panic(code) }

	osUserHomeDir = func() (string, error) { return "/home/test", nil }

	var mkdirPath string
	osMkdirAll = func(path string, perm fs.FileMode) error {
		mkdirPath = path
		return nil
	}

	var writePath string
	var writeData []byte
	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		writePath = name
		writeData = data
		return nil
	}

	jiraConfigCmd([]string{"--init"})

	if mkdirPath != "/home/test/.config/wt" {
		t.Fatalf("expected mkdir /home/test/.config/wt, got %q", mkdirPath)
	}
	if writePath != "/home/test/.config/wt/config.json" {
		t.Fatalf("expected write to /home/test/.config/wt/config.json, got %q", writePath)
	}
	var cfg wtConfig
	if err := json.Unmarshal(writeData, &cfg); err != nil {
		t.Fatalf("invalid JSON written: %v", err)
	}
	if !hasStatusConfig(cfg) {
		t.Fatal("expected valid status config in written data")
	}
	if !strings.Contains(buf.String(), "wrote /home/test/.config/wt/config.json") {
		t.Fatalf("expected wrote message, got %q", buf.String())
	}
}

func TestJiraConfigCmdInitRepo(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	oldExec := execCommand
	oldWriteFile := osWriteFile
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
		execCommand = oldExec
		osWriteFile = oldWriteFile
	}()

	var buf bytes.Buffer
	stdout = &buf
	stdin = strings.NewReader("r\n")
	exitFunc = func(code int) { panic(code) }

	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "-C" {
			args = args[2:]
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return cmdWithOutput("/my/repo")
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	var writePath string
	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		writePath = name
		return nil
	}

	jiraConfigCmd([]string{"--init"})

	if writePath != "/my/repo/.wt.json" {
		t.Fatalf("expected write to /my/repo/.wt.json, got %q", writePath)
	}
	if !strings.Contains(buf.String(), "wrote /my/repo/.wt.json") {
		t.Fatalf("expected wrote message, got %q", buf.String())
	}
}

func TestJiraConfigCmdInitInvalidChoice(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
	}()

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	stdin = strings.NewReader("x\n")
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "invalid choice") {
			t.Fatalf("expected invalid choice error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"--init"})
}

func TestJiraConfigCmdInitNoInput(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
	}()

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	stdin = strings.NewReader("")
	exitFunc = func(code int) { panic(code) }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "no input") {
			t.Fatalf("expected no input error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"--init"})
}

func TestJiraConfigCmdInitWriteError(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	oldHomeDir := osUserHomeDir
	oldMkdir := osMkdirAll
	oldWriteFile := osWriteFile
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
		osUserHomeDir = oldHomeDir
		osMkdirAll = oldMkdir
		osWriteFile = oldWriteFile
	}()

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	stdin = strings.NewReader("g\n")
	exitFunc = func(code int) { panic(code) }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osMkdirAll = func(path string, perm fs.FileMode) error { return nil }
	osWriteFile = func(name string, data []byte, perm fs.FileMode) error {
		return errors.New("disk full")
	}

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "disk full") {
			t.Fatalf("expected disk full error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"--init"})
}

func TestJiraConfigCmdInitMkdirError(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	oldHomeDir := osUserHomeDir
	oldMkdir := osMkdirAll
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
		osUserHomeDir = oldHomeDir
		osMkdirAll = oldMkdir
	}()

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	stdin = strings.NewReader("g\n")
	exitFunc = func(code int) { panic(code) }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osMkdirAll = func(path string, perm fs.FileMode) error {
		return errors.New("permission denied")
	}

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "permission denied") {
			t.Fatalf("expected permission denied error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"--init"})
}

func TestJiraConfigCmdInitHomeDirError(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	oldHomeDir := osUserHomeDir
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
		osUserHomeDir = oldHomeDir
	}()

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	stdin = strings.NewReader("g\n")
	exitFunc = func(code int) { panic(code) }
	osUserHomeDir = func() (string, error) { return "", errors.New("no home") }

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
		if !strings.Contains(errBuf.String(), "no home") {
			t.Fatalf("expected no home error, got %q", errBuf.String())
		}
	}()

	jiraConfigCmd([]string{"--init"})
}

func TestJiraConfigCmdInitRepoRootError(t *testing.T) {
	oldOut := stdout
	oldIn := stdin
	oldExit := exitFunc
	oldErr := stderr
	oldExec := execCommand
	defer func() {
		stdout = oldOut
		stdin = oldIn
		exitFunc = oldExit
		stderr = oldErr
		execCommand = oldExec
	}()

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf
	stdin = strings.NewReader("r\n")
	exitFunc = func(code int) { panic(code) }
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "echo 'not a repo' >&2; exit 128")
	}

	defer func() {
		if r := recover(); r != 1 {
			t.Fatalf("expected exit 1, got %v", r)
		}
	}()

	jiraConfigCmd([]string{"--init"})
}

func TestJiraNewCmdNoConfigDies(t *testing.T) {
	repo := t.TempDir()

	oldGetenv := osGetenv
	oldJiraGet := jiraGet
	oldExec := execCommand
	oldWriteFile := osWriteFile
	oldOut := stdout
	oldErr := stderr
	oldExit := exitFunc
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldJiraGet
		execCommand = oldExec
		osWriteFile = oldWriteFile
		stdout = oldOut
		stderr = oldErr
		exitFunc = oldExit
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

	issue := jiraIssue{Key: "PROJ-123", Fields: jiraFields{Summary: "Fix login"}}
	body, _ := json.Marshal(issue)
	jiraGet = func(url, user, token string) ([]byte, error) { return body, nil }

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

	osWriteFile = func(name string, data []byte, perm fs.FileMode) error { return nil }
	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }

	var buf bytes.Buffer
	stdout = &buf
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

	jiraNewCmd([]string{"PROJ-123"})
}

func TestJiraStatusCmdShowHint(t *testing.T) {
	oldGetenv := osGetenv
	oldGet := jiraGet
	oldOut := stdout
	oldErr := stderr
	oldReadFile := osReadFile
	oldHomeDir := osUserHomeDir
	oldExec := execCommand
	defer func() {
		osGetenv = oldGetenv
		jiraGet = oldGet
		stdout = oldOut
		stderr = oldErr
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
	tr := jiraTransitionsResponse{Transitions: []jiraTransition{}}
	trBody, _ := json.Marshal(tr)

	jiraGet = func(url, user, token string) ([]byte, error) {
		if strings.Contains(url, "/transitions") {
			return trBody, nil
		}
		return issueBody, nil
	}

	osUserHomeDir = func() (string, error) { return "/home/test", nil }
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrNotExist }
	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", "exit 1")
	}

	var buf bytes.Buffer
	stdout = &buf
	var errBuf bytes.Buffer
	stderr = &errBuf

	jiraStatusCmd([]string{"PROJ-123"})

	if !strings.Contains(errBuf.String(), "hint: run 'wt jira config --init'") {
		t.Fatalf("expected hint on stderr, got %q", errBuf.String())
	}
}
