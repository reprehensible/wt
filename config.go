package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	osReadFile    = os.ReadFile
	osUserHomeDir = os.UserHomeDir
)

type wtConfig struct {
	Jira jiraConfigBlock `json:"jira"`
}

type jiraConfigBlock struct {
	Status jiraStatusConfig `json:"status"`
}

type jiraStatusConfig struct {
	Default map[string]string            `json:"default"`
	Types   map[string]map[string]string `json:"types"`
}

func loadConfig() (wtConfig, error) {
	var global wtConfig
	var repo wtConfig
	globalFound := false
	repoFound := false

	home, err := osUserHomeDir()
	if err == nil {
		globalPath := filepath.Join(home, ".config", "wt", "config.json")
		data, err := osReadFile(globalPath)
		if err == nil {
			if err := json.Unmarshal(data, &global); err != nil {
				return wtConfig{}, fmt.Errorf("invalid config %s: %w", globalPath, err)
			}
			globalFound = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return wtConfig{}, err
		}
	}

	root, err := gitRepoRoot()
	if err == nil {
		repoPath := filepath.Join(root, ".wt.json")
		data, err := osReadFile(repoPath)
		if err == nil {
			if err := json.Unmarshal(data, &repo); err != nil {
				return wtConfig{}, fmt.Errorf("invalid config %s: %w", repoPath, err)
			}
			repoFound = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return wtConfig{}, err
		}
	}

	if !globalFound && !repoFound {
		return wtConfig{}, nil
	}
	if !repoFound {
		return global, nil
	}
	if !globalFound {
		return repo, nil
	}
	return mergeConfig(global, repo), nil
}

func mergeConfig(global, repo wtConfig) wtConfig {
	merged := global

	if merged.Jira.Status.Default == nil {
		merged.Jira.Status.Default = make(map[string]string)
	}
	for k, v := range repo.Jira.Status.Default {
		merged.Jira.Status.Default[k] = v
	}

	if merged.Jira.Status.Types == nil {
		merged.Jira.Status.Types = make(map[string]map[string]string)
	}
	for typeName, overrides := range repo.Jira.Status.Types {
		if merged.Jira.Status.Types[typeName] == nil {
			merged.Jira.Status.Types[typeName] = make(map[string]string)
		}
		for k, v := range overrides {
			merged.Jira.Status.Types[typeName][k] = v
		}
	}

	return merged
}

func reverseSymbolic(cfg wtConfig, issueType, jiraStatusName string) string {
	lower := strings.ToLower(issueType)
	if m, ok := cfg.Jira.Status.Types[lower]; ok {
		for k, v := range m {
			if strings.EqualFold(v, jiraStatusName) {
				return k
			}
		}
	}
	if m := cfg.Jira.Status.Default; m != nil {
		for k, v := range m {
			if strings.EqualFold(v, jiraStatusName) {
				return k
			}
		}
	}
	return ""
}

func hasStatusConfig(cfg wtConfig) bool {
	return len(cfg.Jira.Status.Default) > 0 || len(cfg.Jira.Status.Types) > 0
}

func templateConfig() wtConfig {
	return wtConfig{Jira: jiraConfigBlock{Status: jiraStatusConfig{
		Default: map[string]string{
			"working": "In Progress",
			"review":  "In Review",
			"testing": "Testing",
			"done":    "Done",
		},
	}}}
}

func resolveStatus(cfg wtConfig, issueType, symbolic string) (string, error) {
	lower := strings.ToLower(issueType)
	if m, ok := cfg.Jira.Status.Types[lower]; ok {
		if v, ok := m[symbolic]; ok {
			return v, nil
		}
	}
	if m := cfg.Jira.Status.Default; m != nil {
		if v, ok := m[symbolic]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("no status mapping for %q", symbolic)
}
