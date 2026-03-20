package git

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultTimeout = 60 * time.Second

type Client struct {
	timeout time.Duration
}

func New() *Client {
	return &Client{timeout: defaultTimeout}
}

func (c *Client) run(cwd string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output != "" {
			return output, fmt.Errorf("%s: %s", err, output)
		}
		return "", err
	}
	return output, nil
}

func (c *Client) stdout(cwd string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	out, err := cmd.Output()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if stderr != "" {
				return output, fmt.Errorf("%s: %s", err, stderr)
			}
		}
		return output, err
	}
	return output, nil
}

func (c *Client) Root(cwd string) (string, error) {
	return c.stdout(cwd, "git", "rev-parse", "--show-toplevel")
}

func (c *Client) CurrentBranch(cwd string) (string, error) {
	return c.stdout(cwd, "git", "rev-parse", "--abbrev-ref", "HEAD")
}

func (c *Client) HasUncommittedChanges(cwd string) bool {
	out, err := c.stdout(cwd, "git", "status", "--porcelain")
	return err == nil && out != ""
}

func (c *Client) HasStagedChanges(cwd string) bool {
	_, err := c.stdout(cwd, "git", "diff", "--cached", "--quiet")
	return err != nil // exit code 1 means there ARE staged changes
}

func (c *Client) Stage(cwd string, files []string) error {
	if len(files) == 0 {
		_, err := c.run(cwd, "git", "add", "-A")
		return err
	}
	args := append([]string{"git", "add", "--"}, files...)
	_, err := c.run(cwd, args...)
	return err
}

func (c *Client) Commit(cwd, message string) error {
	_, err := c.run(cwd, "git", "commit", "-m", message)
	return err
}

func (c *Client) Push(cwd, branch string) error {
	_, err := c.run(cwd, "git", "push", "-u", "origin", branch)
	return err
}

func (c *Client) CheckoutAndPull(cwd, branch string) error {
	if _, err := c.run(cwd, "git", "checkout", branch); err != nil {
		return err
	}
	_, err := c.run(cwd, "git", "pull", "--ff-only")
	return err
}

func (c *Client) CreateBranch(cwd, name, base string) error {
	_, err := c.run(cwd, "git", "checkout", "-b", name, base)
	return err
}

func (c *Client) Checkout(cwd, branch string) error {
	_, err := c.run(cwd, "git", "checkout", branch)
	return err
}

func (c *Client) DeleteLocalBranch(cwd, name string) error {
	_, err := c.run(cwd, "git", "branch", "-d", name)
	return err
}

func (c *Client) PruneRemotes(cwd string) error {
	_, err := c.run(cwd, "git", "remote", "prune", "origin")
	return err
}

func (c *Client) Fetch(cwd, remote, ref string) error {
	_, err := c.run(cwd, "git", "fetch", remote, ref)
	return err
}

func (c *Client) RemoteURL(cwd string) (string, error) {
	return c.stdout(cwd, "git", "remote", "get-url", "origin")
}

func (c *Client) IsBehindRemote(cwd, branch string) bool {
	_ = c.Fetch(cwd, "origin", branch)
	out, err := c.stdout(cwd, "git", "rev-list", "--count", branch+"..origin/"+branch)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != "0"
}

func (c *Client) MergedBranches(cwd, base string, remote bool) ([]string, error) {
	target := base
	if remote {
		_ = c.Fetch(cwd, "origin", base)
		target = "origin/" + base
	}

	out, err := c.stdout(cwd, "git", "branch", "--merged", target)
	if err != nil {
		return nil, err
	}

	current, _ := c.CurrentBranch(cwd)
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "* "))
		if name == "" || name == "dev" || name == "main" || name == current {
			continue
		}
		branches = append(branches, name)
	}
	return branches, nil
}

func (c *Client) ListRemoteBranches(cwd string) ([]string, error) {
	out, err := c.stdout(cwd, "git", "ls-remote", "--heads", "origin")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ref := strings.TrimPrefix(parts[1], "refs/heads/")
		branches = append(branches, ref)
	}
	return branches, nil
}

func (c *Client) LatestTag(cwd string) (string, error) {
	out, err := c.stdout(cwd, "git", "describe", "--tags", "--abbrev=0")
	if err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) CommitsSinceTag(cwd, tag string) ([]string, error) {
	ref := tag + "..HEAD"
	if tag == "" {
		ref = "HEAD"
	}
	out, err := c.stdout(cwd, "git", "log", ref, "--oneline", "--no-merges")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (c *Client) CommitCountBetween(cwd, from, to string) (int, error) {
	out, err := c.stdout(cwd, "git", "rev-list", "--count", from+".."+to)
	if err != nil {
		return 0, err
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &n)
	return n, nil
}

func (c *Client) MergeBranch(cwd, from string) error {
	_, err := c.run(cwd, "git", "merge", from, "--no-edit")
	return err
}

func (c *Client) Tag(cwd, tag string) error {
	_, err := c.run(cwd, "git", "tag", tag)
	return err
}

func (c *Client) PushTag(cwd, tag string) error {
	_, err := c.run(cwd, "git", "push", "origin", tag)
	return err
}

func (c *Client) DeleteRemoteBranch(cwd, branch string) error {
	_, err := c.run(cwd, "git", "push", "origin", "--delete", branch)
	return err
}

func (c *Client) RemoteBranchExists(cwd, branch string) bool {
	out, err := c.stdout(cwd, "git", "ls-remote", "--heads", "origin", branch)
	return err == nil && strings.Contains(out, branch)
}
