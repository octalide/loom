package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v84/github"
)

type Issue struct {
	Number    int
	Title     string
	Body      string
	URL       string
	State     string
	Labels    []string
	Assignees []string
	Comments  []IssueComment
	CreatedAt string
	UpdatedAt string
}

type IssueComment struct {
	Author    string
	Body      string
	CreatedAt string
}

func (c *Client) CreateIssue(ctx context.Context, repo, title, body string) (number int, url string, err error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return 0, "", err
	}
	issue, _, err := c.REST.Issues.Create(ctx, owner, name, &gh.IssueRequest{
		Title: gh.Ptr(title),
		Body:  gh.Ptr(body),
	})
	if err != nil {
		return 0, "", fmt.Errorf("create issue: %w", err)
	}
	return issue.GetNumber(), issue.GetHTMLURL(), nil
}

func (c *Client) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	issue, _, err := c.REST.Issues.Get(ctx, owner, name, number)
	if err != nil {
		return nil, fmt.Errorf("get issue #%d: %w", number, err)
	}

	var labels []string
	for _, l := range issue.Labels {
		labels = append(labels, l.GetName())
	}
	var assignees []string
	for _, a := range issue.Assignees {
		assignees = append(assignees, a.GetLogin())
	}

	return &Issue{
		Number:    issue.GetNumber(),
		Title:     issue.GetTitle(),
		Body:      issue.GetBody(),
		URL:       issue.GetHTMLURL(),
		State:     issue.GetState(),
		Labels:    labels,
		Assignees: assignees,
		CreatedAt: issue.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: issue.GetUpdatedAt().Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (c *Client) GetIssueComments(ctx context.Context, repo string, number int) ([]IssueComment, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	comments, _, err := c.REST.Issues.ListComments(ctx, owner, name, number, &gh.IssueListCommentsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("list comments for #%d: %w", number, err)
	}

	var result []IssueComment
	for _, c := range comments {
		result = append(result, IssueComment{
			Author:    c.GetUser().GetLogin(),
			Body:      c.GetBody(),
			CreatedAt: c.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

func (c *Client) ListOpenIssues(ctx context.Context, repo string) ([]*Issue, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	ghIssues, _, err := c.REST.Issues.ListByRepo(ctx, owner, name, &gh.IssueListByRepoOptions{
		State:       "open",
		ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("list open issues: %w", err)
	}
	var result []*Issue
	for _, issue := range ghIssues {
		if issue.IsPullRequest() {
			continue
		}
		var labels []string
		for _, l := range issue.Labels {
			labels = append(labels, l.GetName())
		}
		result = append(result, &Issue{
			Number:    issue.GetNumber(),
			Title:     issue.GetTitle(),
			Body:      issue.GetBody(),
			URL:       issue.GetHTMLURL(),
			State:     issue.GetState(),
			Labels:    labels,
			CreatedAt: issue.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
			UpdatedAt: issue.GetUpdatedAt().Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

func (c *Client) AddLabels(ctx context.Context, repo string, number int, labels []string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, _, err = c.REST.Issues.AddLabelsToIssue(ctx, owner, name, number, labels)
	return err
}

func (c *Client) GetIssueURL(ctx context.Context, repo string, number int) (string, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return "", err
	}
	issue, _, err := c.REST.Issues.Get(ctx, owner, name, number)
	if err != nil {
		return "", err
	}
	return issue.GetHTMLURL(), nil
}
