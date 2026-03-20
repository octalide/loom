package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v84/github"
)

type Label struct {
	Name        string
	Description string
	Color       string // hex without '#'
}

var DefaultLabels = []Label{
	{Name: "feat", Description: "New feature", Color: "1f883d"},
	{Name: "fix", Description: "Bug fix", Color: "d73a4a"},
	{Name: "refactor", Description: "Code restructuring", Color: "1d76db"},
	{Name: "doc", Description: "Documentation", Color: "a371f7"},
	{Name: "test", Description: "Tests", Color: "e3b341"},
	{Name: "chore", Description: "Maintenance", Color: "6e7781"},
	{Name: "build", Description: "Build system", Color: "e16f24"},
	{Name: "ci", Description: "CI/CD", Color: "e16f24"},
	{Name: "perf", Description: "Performance", Color: "1d76db"},
}

func (c *Client) ListLabels(ctx context.Context, repo string) ([]Label, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}

	var all []Label
	opts := &gh.ListOptions{PerPage: 100}
	for {
		labels, resp, err := c.REST.Issues.ListLabels(ctx, owner, name, opts)
		if err != nil {
			return nil, fmt.Errorf("list labels: %w", err)
		}
		for _, l := range labels {
			all = append(all, Label{
				Name:        l.GetName(),
				Description: l.GetDescription(),
				Color:       l.GetColor(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

func (c *Client) CreateLabel(ctx context.Context, repo string, label Label) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, _, err = c.REST.Issues.CreateLabel(ctx, owner, name, &gh.Label{
		Name:        gh.Ptr(label.Name),
		Description: gh.Ptr(label.Description),
		Color:       gh.Ptr(label.Color),
	})
	if err != nil {
		return fmt.Errorf("create label %q: %w", label.Name, err)
	}
	return nil
}

func (c *Client) DeleteLabel(ctx context.Context, repo, labelName string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, err = c.REST.Issues.DeleteLabel(ctx, owner, name, labelName)
	if err != nil {
		return fmt.Errorf("delete label %q: %w", labelName, err)
	}
	return nil
}

// EnsureLabels creates any labels from the list that don't already exist.
// Returns the names of labels that were created.
func (c *Client) EnsureLabels(ctx context.Context, repo string, labels []Label) (created []string, err error) {
	existing, err := c.ListLabels(ctx, repo)
	if err != nil {
		return nil, err
	}

	existingSet := make(map[string]bool)
	for _, l := range existing {
		existingSet[l.Name] = true
	}

	for _, l := range labels {
		if !existingSet[l.Name] {
			if err := c.CreateLabel(ctx, repo, l); err != nil {
				return created, err
			}
			created = append(created, l.Name)
		}
	}

	return created, nil
}
