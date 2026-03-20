package github

import (
	"context"
	"fmt"
	"strings"
	"sync"

	gh "github.com/google/go-github/v84/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

type Client struct {
	REST    *gh.Client
	GraphQL *githubv4.Client
	token   string

	userOnce sync.Once
	userName string
}

func NewClient(token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), ts)

	return &Client{
		REST:    gh.NewClient(httpClient),
		GraphQL: githubv4.NewClient(httpClient),
		token:   token,
	}
}

// AuthenticatedUser returns the login of the current user (cached).
func (c *Client) AuthenticatedUser(ctx context.Context) (string, error) {
	var err error
	c.userOnce.Do(func() {
		var user *gh.User
		user, _, err = c.REST.Users.Get(ctx, "")
		if err == nil {
			c.userName = user.GetLogin()
		}
	})
	if err != nil {
		return "", fmt.Errorf("get authenticated user: %w", err)
	}
	return c.userName, nil
}

// Token returns the token used by this client.
func (c *Client) Token() string {
	return c.token
}

// SplitRepo splits "owner/repo" into owner and repo name.
func SplitRepo(repo string) (owner, name string, err error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo format %q; expected owner/repo", repo)
	}
	return parts[0], parts[1], nil
}
