package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Project  int            `yaml:"project"`
	Repo     string         `yaml:"repo"`
	Branches BranchesConfig `yaml:"branches"`
	Statuses StatusesConfig `yaml:"statuses"`
	Checks   []string       `yaml:"checks"`
}

type BranchesConfig struct {
	Base    string   `yaml:"base"`
	Release string   `yaml:"release"`
	Types   []string `yaml:"types"`
}

type StatusesConfig struct {
	Todo       string `yaml:"todo"`
	InProgress string `yaml:"in_progress"`
	Done       string `yaml:"done"`
}

func Default() *Config {
	return &Config{
		Branches: BranchesConfig{
			Base:    "dev",
			Release: "main",
			Types:   []string{"feat", "fix", "doc", "refactor", "issue"},
		},
		Statuses: StatusesConfig{
			Todo:       "Todo",
			InProgress: "In Progress",
			Done:       "Done",
		},
	}
}

// Load reads .github/loom.yml from the given git root directory.
// Returns defaults if the file doesn't exist.
func Load(gitRoot string) *Config {
	cfg := Default()
	if gitRoot == "" {
		return cfg
	}

	path := filepath.Join(gitRoot, ".github", "loom.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	var file Config
	if err := yaml.Unmarshal(data, &file); err != nil {
		return cfg
	}

	merge(cfg, &file)
	return cfg
}

func merge(dst, src *Config) {
	if src.Project != 0 {
		dst.Project = src.Project
	}
	if src.Repo != "" {
		dst.Repo = src.Repo
	}
	if src.Branches.Base != "" {
		dst.Branches.Base = src.Branches.Base
	}
	if src.Branches.Release != "" {
		dst.Branches.Release = src.Branches.Release
	}
	if len(src.Branches.Types) > 0 {
		dst.Branches.Types = src.Branches.Types
	}
	if src.Statuses.Todo != "" {
		dst.Statuses.Todo = src.Statuses.Todo
	}
	if src.Statuses.InProgress != "" {
		dst.Statuses.InProgress = src.Statuses.InProgress
	}
	if src.Statuses.Done != "" {
		dst.Statuses.Done = src.Statuses.Done
	}
	if len(src.Checks) > 0 {
		dst.Checks = src.Checks
	}
}

// ValidBranchType checks if a branch type prefix is allowed by the config.
func (c *Config) ValidBranchType(t string) bool {
	for _, allowed := range c.Branches.Types {
		if allowed == t {
			return true
		}
	}
	return false
}
