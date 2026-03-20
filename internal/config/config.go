package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Repo        string         `yaml:"repo"`
	MergeMethod string         `yaml:"merge_method"`
	Strict      *bool          `yaml:"strict,omitempty"`
	Branches    BranchesConfig `yaml:"branches"`
	Checks      []string       `yaml:"checks"`
}

type BranchesConfig struct {
	Base    string   `yaml:"base"`
	Release string   `yaml:"release"`
	Types   []string `yaml:"types"`
}

func Default() *Config {
	return &Config{
		MergeMethod: "MERGE",
		Branches: BranchesConfig{
			Base:    "dev",
			Release: "main",
			Types:   []string{"feat", "fix", "doc", "refactor", "issue"},
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
	if src.MergeMethod != "" {
		dst.MergeMethod = normalizeMergeMethod(src.MergeMethod)
	}
	if src.Strict != nil {
		dst.Strict = src.Strict
	}
	if len(src.Checks) > 0 {
		dst.Checks = src.Checks
	}
}

func normalizeMergeMethod(m string) string {
	switch strings.ToUpper(m) {
	case "SQUASH":
		return "SQUASH"
	case "REBASE":
		return "REBASE"
	default:
		return "MERGE"
	}
}

// IsStrict returns whether branch protection should require branches to be up to date.
// Defaults to false.
func (c *Config) IsStrict() bool {
	if c.Strict != nil {
		return *c.Strict
	}
	return false
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
