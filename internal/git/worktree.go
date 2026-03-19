package git

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Worktree struct {
	Path     string
	Branch   string
	Head     string
	Bare     bool
	Detached bool
}

func (c *Client) IsWorktree(cwd string) bool {
	gitDir, err := c.stdout(cwd, "git", "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	gitCommon, err := c.stdout(cwd, "git", "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}
	realGitDir, _ := filepath.EvalSymlinks(gitDir)
	realCommon, _ := filepath.EvalSymlinks(gitCommon)
	return realGitDir != realCommon
}

func (c *Client) MainWorktree(cwd string) (string, error) {
	out, err := c.stdout(cwd, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			return strings.TrimPrefix(line, "worktree "), nil
		}
	}
	return "", fmt.Errorf("no main worktree found")
}

func (c *Client) WorktreePathForBranch(cwd, branch string) (string, error) {
	main, err := c.MainWorktree(cwd)
	if err != nil {
		root, rootErr := c.Root(cwd)
		if rootErr != nil {
			return "", fmt.Errorf("cannot determine repo root: %w", rootErr)
		}
		main = root
	}
	dirname := filepath.Base(main) + "--" + strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(filepath.Dir(main), dirname), nil
}

func (c *Client) CreateWorktree(cwd, path, branch, base string) error {
	_, err := c.run(cwd, "git", "worktree", "add", path, "-b", branch, base)
	return err
}

func (c *Client) AddWorktree(cwd, path, branch string) error {
	_, err := c.run(cwd, "git", "worktree", "add", path, branch)
	return err
}

func (c *Client) RemoveWorktree(cwd, path string) error {
	_, err := c.run(cwd, "git", "worktree", "remove", path, "--force")
	return err
}

var issueRe = regexp.MustCompile(`/(\d+)$`)

func (c *Client) ListWorktrees(cwd string) ([]Worktree, error) {
	out, err := c.stdout(cwd, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			if idx := strings.Index(ref, "refs/heads/"); idx >= 0 {
				ref = ref[idx+len("refs/heads/"):]
			}
			current.Branch = ref
		case line == "bare":
			current.Bare = true
		case line == "detached":
			current.Detached = true
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees, nil
}

func (c *Client) WorktreeExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
