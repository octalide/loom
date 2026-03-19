package github

import (
	"context"
	"fmt"

	"github.com/shurcooL/githubv4"
)

type Dependency struct {
	Number int
	Title  string
	State  string
	Repo   string
}

type Dependencies struct {
	BlockedBy []Dependency
	Blocking  []Dependency
	Parent    *Dependency
	SubIssues []Dependency
}

func (c *Client) AddBlockingLink(ctx context.Context, repo string, issue int, targetRepo string, targetIssue int, relationship string) error {
	sourceID, err := c.issueNodeID(ctx, repo, issue)
	if err != nil {
		return err
	}
	targetID, err := c.issueNodeID(ctx, targetRepo, targetIssue)
	if err != nil {
		return err
	}

	var blockedID, blockingID string
	switch relationship {
	case "blocked_by":
		blockedID, blockingID = sourceID, targetID
	case "blocks":
		blockedID, blockingID = targetID, sourceID
	default:
		return fmt.Errorf("invalid blocking relationship: %q", relationship)
	}

	var mutation struct {
		AddBlockedBy struct {
			ClientMutationID string
		} `graphql:"addBlockedBy(input: $input)"`
	}
	type AddBlockedByInput struct {
		IssueID         string `json:"issueId"`
		BlockingIssueID string `json:"blockingIssueId"`
	}
	return c.GraphQL.Mutate(ctx, &mutation, AddBlockedByInput{
		IssueID:         blockedID,
		BlockingIssueID: blockingID,
	}, nil)
}

func (c *Client) RemoveBlockingLink(ctx context.Context, repo string, issue int, targetRepo string, targetIssue int, relationship string) error {
	sourceID, err := c.issueNodeID(ctx, repo, issue)
	if err != nil {
		return err
	}
	targetID, err := c.issueNodeID(ctx, targetRepo, targetIssue)
	if err != nil {
		return err
	}

	var blockedID, blockingID string
	switch relationship {
	case "blocked_by":
		blockedID, blockingID = sourceID, targetID
	case "blocks":
		blockedID, blockingID = targetID, sourceID
	default:
		return fmt.Errorf("invalid blocking relationship: %q", relationship)
	}

	var mutation struct {
		RemoveBlockedBy struct {
			ClientMutationID string
		} `graphql:"removeBlockedBy(input: $input)"`
	}
	type RemoveBlockedByInput struct {
		IssueID         string `json:"issueId"`
		BlockingIssueID string `json:"blockingIssueId"`
	}
	return c.GraphQL.Mutate(ctx, &mutation, RemoveBlockedByInput{
		IssueID:         blockedID,
		BlockingIssueID: blockingID,
	}, nil)
}

func (c *Client) AddSubIssueLink(ctx context.Context, repo string, issue int, targetRepo string, targetIssue int, relationship string) error {
	var parentRepo string
	var parentNumber, childNumber int

	switch relationship {
	case "parent_of":
		parentRepo, parentNumber = repo, issue
		_, childNumber = targetRepo, targetIssue
	case "child_of":
		parentRepo, parentNumber = targetRepo, targetIssue
		_, childNumber = repo, issue
	default:
		return fmt.Errorf("invalid sub-issue relationship: %q", relationship)
	}

	owner, name, err := SplitRepo(parentRepo)
	if err != nil {
		return err
	}

	// Get the child's REST ID
	childOwner, childName, err := SplitRepo(func() string {
		if relationship == "child_of" {
			return repo
		}
		return targetRepo
	}())
	if err != nil {
		return err
	}

	childIssue, _, err := c.REST.Issues.Get(ctx, childOwner, childName, childNumber)
	if err != nil {
		return fmt.Errorf("get child issue: %w", err)
	}

	// Use the sub-issues REST API
	url := fmt.Sprintf("repos/%s/%s/issues/%d/sub_issues", owner, name, parentNumber)
	req, err := c.REST.NewRequest("POST", url, map[string]interface{}{
		"sub_issue_id": childIssue.GetID(),
	})
	if err != nil {
		return fmt.Errorf("create sub-issue request: %w", err)
	}
	_, err = c.REST.Do(ctx, req, nil)
	if err != nil {
		return fmt.Errorf("add sub-issue: %w", err)
	}
	return nil
}

func (c *Client) RemoveSubIssueLink(ctx context.Context, repo string, issue int, targetRepo string, targetIssue int, relationship string) error {
	var parentRepo string
	var parentNumber, childNumber int

	switch relationship {
	case "parent_of":
		parentRepo, parentNumber = repo, issue
		_, childNumber = targetRepo, targetIssue
	case "child_of":
		parentRepo, parentNumber = targetRepo, targetIssue
		_, childNumber = repo, issue
	default:
		return fmt.Errorf("invalid sub-issue relationship: %q", relationship)
	}

	owner, name, err := SplitRepo(parentRepo)
	if err != nil {
		return err
	}

	childOwner, childName, err := SplitRepo(func() string {
		if relationship == "child_of" {
			return repo
		}
		return targetRepo
	}())
	if err != nil {
		return err
	}

	childIssue, _, err := c.REST.Issues.Get(ctx, childOwner, childName, childNumber)
	if err != nil {
		return fmt.Errorf("get child issue: %w", err)
	}

	url := fmt.Sprintf("repos/%s/%s/issues/%d/sub_issue", owner, name, parentNumber)
	req, err := c.REST.NewRequest("DELETE", url, map[string]interface{}{
		"sub_issue_id": childIssue.GetID(),
	})
	if err != nil {
		return fmt.Errorf("create remove sub-issue request: %w", err)
	}
	_, err = c.REST.Do(ctx, req, nil)
	if err != nil {
		return fmt.Errorf("remove sub-issue: %w", err)
	}
	return nil
}

func (c *Client) GetDependencies(ctx context.Context, repo string, issueNumber int) (*Dependencies, error) {
	nodeID, err := c.issueNodeID(ctx, repo, issueNumber)
	if err != nil {
		return nil, err
	}

	// Query blocking relationships via GraphQL
	var query struct {
		Node struct {
			Issue struct {
				BlockedBy struct {
					Nodes []struct {
						Number     int
						Title      string
						State      string
						Repository struct {
							NameWithOwner string
						}
					}
				} `graphql:"blockedBy(first: 50)"`
				Blocking struct {
					Nodes []struct {
						Number     int
						Title      string
						State      string
						Repository struct {
							NameWithOwner string
						}
					}
				} `graphql:"blocking(first: 50)"`
			} `graphql:"... on Issue"`
		} `graphql:"node(id: $id)"`
	}
	vars := map[string]interface{}{"id": githubv4.ID(nodeID)}
	if err := c.GraphQL.Query(ctx, &query, vars); err != nil {
		return nil, fmt.Errorf("query blocking relationships: %w", err)
	}

	deps := &Dependencies{}
	for _, n := range query.Node.Issue.BlockedBy.Nodes {
		deps.BlockedBy = append(deps.BlockedBy, Dependency{
			Number: n.Number, Title: n.Title, State: n.State,
			Repo: n.Repository.NameWithOwner,
		})
	}
	for _, n := range query.Node.Issue.Blocking.Nodes {
		deps.Blocking = append(deps.Blocking, Dependency{
			Number: n.Number, Title: n.Title, State: n.State,
			Repo: n.Repository.NameWithOwner,
		})
	}

	// Query sub-issues via REST
	owner, name, err := SplitRepo(repo)
	if err == nil {
		deps.SubIssues = c.getSubIssues(ctx, owner, name, issueNumber)
		deps.Parent = c.getParentIssue(ctx, owner, name, issueNumber, repo)
	}

	return deps, nil
}

func (c *Client) getSubIssues(ctx context.Context, owner, name string, issueNumber int) []Dependency {
	url := fmt.Sprintf("repos/%s/%s/issues/%d/sub_issues", owner, name, issueNumber)
	req, err := c.REST.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	var subIssues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
	}
	_, err = c.REST.Do(ctx, req, &subIssues)
	if err != nil {
		return nil
	}
	var deps []Dependency
	for _, s := range subIssues {
		deps = append(deps, Dependency{
			Number: s.Number, Title: s.Title, State: s.State,
			Repo: owner + "/" + name,
		})
	}
	return deps
}

func (c *Client) getParentIssue(ctx context.Context, owner, name string, issueNumber int, repo string) *Dependency {
	issue, _, err := c.REST.Issues.Get(ctx, owner, name, issueNumber)
	if err != nil {
		return nil
	}
	// The parent field is available in the issue response if the issue has a parent
	// Check via the raw JSON since go-github may not have this field yet
	_ = issue
	// For now, fall back to the REST API endpoint
	url := fmt.Sprintf("repos/%s/%s/issues/%d", owner, name, issueNumber)
	req, err := c.REST.NewRequest("GET", url, nil)
	if err != nil {
		return nil
	}
	var issueData struct {
		Parent *struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
		} `json:"parent"`
	}
	_, err = c.REST.Do(ctx, req, &issueData)
	if err != nil || issueData.Parent == nil {
		return nil
	}
	return &Dependency{
		Number: issueData.Parent.Number,
		Title:  issueData.Parent.Title,
		State:  issueData.Parent.State,
		Repo:   repo,
	}
}

func (c *Client) issueNodeID(ctx context.Context, repo string, number int) (string, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return "", err
	}
	issue, _, err := c.REST.Issues.Get(ctx, owner, name, number)
	if err != nil {
		return "", fmt.Errorf("get issue #%d node ID: %w", number, err)
	}
	return issue.GetNodeID(), nil
}
