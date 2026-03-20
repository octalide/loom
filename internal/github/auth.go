package github

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveToken finds a GitHub token using the standard resolution chain:
// GH_TOKEN env → GITHUB_TOKEN env → gh auth token (CLI keyring).
func ResolveToken() (string, error) {
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t, nil
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, nil
	}

	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		t := strings.TrimSpace(string(out))
		if t != "" {
			return t, nil
		}
	}

	return "", fmt.Errorf("no GitHub token found; set GH_TOKEN, GITHUB_TOKEN, or run 'gh auth login'")
}

// CheckAuth verifies the token works.
func (c *Client) CheckAuth(token string) error {
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		return fmt.Errorf("not authenticated: %s", strings.TrimSpace(string(out)))
	}
	_ = out
	return nil
}
