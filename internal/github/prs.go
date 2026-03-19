package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v84/github"
)

type PullRequest struct {
	Number       int
	Title        string
	URL          string
	State        string
	IsDraft      bool
	HeadRefName  string
	BaseRefName  string
	Additions    int
	Deletions    int
	ChangedFiles int
}

type ReviewComment struct {
	Author    string
	Body      string
	Path      string
	Line      int
	CreatedAt string
	InReplyTo int64
}

type Review struct {
	Author      string
	State       string
	Body        string
	SubmittedAt string
}

type MergeReadiness struct {
	IsDraft        bool
	Mergeable      string
	MergeState     string
	ReviewDecision string
	AutoMerge      bool
	AutoMergeMethod string
	Checks         []CheckStatus
	Summary        []string
}

type CheckStatus struct {
	Name       string
	Conclusion string
	Status     string
	IsRequired bool
}

func (c *Client) CreateDraftPR(ctx context.Context, repo, title, body, base, head string) (number int, url string, err error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return 0, "", err
	}
	pr, _, err := c.REST.PullRequests.Create(ctx, owner, name, &gh.NewPullRequest{
		Title: gh.Ptr(title),
		Body:  gh.Ptr(body),
		Base:  gh.Ptr(base),
		Head:  gh.Ptr(head),
		Draft: gh.Ptr(true),
	})
	if err != nil {
		return 0, "", fmt.Errorf("create draft PR: %w", err)
	}
	return pr.GetNumber(), pr.GetHTMLURL(), nil
}

func (c *Client) FindPRForBranch(ctx context.Context, repo, branch string) (*PullRequest, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	prs, _, err := c.REST.PullRequests.List(ctx, owner, name, &gh.PullRequestListOptions{
		Head:        owner + ":" + branch,
		State:       "all",
		ListOptions: gh.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, fmt.Errorf("find PR for branch %q: %w", branch, err)
	}
	if len(prs) == 0 {
		return nil, fmt.Errorf("no PR found for branch %q", branch)
	}
	pr := prs[0]
	return prFromREST(pr), nil
}

func (c *Client) GetPR(ctx context.Context, repo string, number int) (*PullRequest, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	pr, _, err := c.REST.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return nil, fmt.Errorf("get PR #%d: %w", number, err)
	}
	return prFromREST(pr), nil
}

func (c *Client) ListOpenPRs(ctx context.Context, repo string) ([]*PullRequest, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	prs, _, err := c.REST.PullRequests.List(ctx, owner, name, &gh.PullRequestListOptions{
		State:       "open",
		ListOptions: gh.ListOptions{PerPage: 50},
	})
	if err != nil {
		return nil, fmt.Errorf("list open PRs: %w", err)
	}
	var result []*PullRequest
	for _, pr := range prs {
		result = append(result, prFromREST(pr))
	}
	return result, nil
}

func (c *Client) ReadyPR(ctx context.Context, repo string, number int) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	// Mark as ready by updating draft to false
	_, _, err = c.REST.PullRequests.Edit(ctx, owner, name, number, &gh.PullRequest{
		Draft: gh.Ptr(false),
	})
	if err != nil {
		return fmt.Errorf("mark PR #%d ready: %w", number, err)
	}
	return nil
}

func (c *Client) EnableAutoMerge(ctx context.Context, repo string, number int) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	// Use GraphQL for auto-merge since REST doesn't support it directly
	pr, _, err := c.REST.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return fmt.Errorf("get PR #%d for auto-merge: %w", number, err)
	}

	var mutation struct {
		EnablePullRequestAutoMerge struct {
			ClientMutationID string
		} `graphql:"enablePullRequestAutoMerge(input: $input)"`
	}
	input := EnablePullRequestAutoMergeInput{
		PullRequestID: pr.GetNodeID(),
		MergeMethod:   "SQUASH",
	}
	err = c.GraphQL.Mutate(ctx, &mutation, input, nil)
	if err != nil {
		return fmt.Errorf("enable auto-merge for PR #%d: %w", number, err)
	}
	return nil
}

type EnablePullRequestAutoMergeInput struct {
	PullRequestID string `json:"pullRequestId"`
	MergeMethod   string `json:"mergeMethod"`
}

func (c *Client) GetPRReviews(ctx context.Context, repo string, number int) ([]Review, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	reviews, _, err := c.REST.PullRequests.ListReviews(ctx, owner, name, number, &gh.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("list reviews for PR #%d: %w", number, err)
	}
	var result []Review
	for _, r := range reviews {
		result = append(result, Review{
			Author:      r.GetUser().GetLogin(),
			State:       r.GetState(),
			Body:        r.GetBody(),
			SubmittedAt: r.GetSubmittedAt().Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

func (c *Client) GetPRReviewComments(ctx context.Context, repo string, number int) (map[string][]ReviewComment, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	comments, _, err := c.REST.PullRequests.ListComments(ctx, owner, name, number, &gh.PullRequestListCommentsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("list review comments for PR #%d: %w", number, err)
	}
	byFile := make(map[string][]ReviewComment)
	for _, c := range comments {
		path := c.GetPath()
		byFile[path] = append(byFile[path], ReviewComment{
			Author:    c.GetUser().GetLogin(),
			Body:      c.GetBody(),
			Path:      path,
			Line:      c.GetLine(),
			CreatedAt: c.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
			InReplyTo: c.GetInReplyTo(),
		})
	}
	return byFile, nil
}

func (c *Client) GetPRComments(ctx context.Context, repo string, number int) ([]IssueComment, error) {
	// PR conversation comments use the Issues API
	return c.GetIssueComments(ctx, repo, number)
}

func (c *Client) FindPRForIssue(ctx context.Context, repo string, issueNumber int) (*PullRequest, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("#%d in:body", issueNumber)
	prs, _, err := c.REST.PullRequests.List(ctx, owner, name, &gh.PullRequestListOptions{
		State:       "all",
		ListOptions: gh.ListOptions{PerPage: 10},
	})
	if err != nil {
		return nil, fmt.Errorf("search PRs for issue #%d: %w", issueNumber, err)
	}
	// Filter manually since the REST list API doesn't support search queries
	_ = query
	for _, pr := range prs {
		body := pr.GetBody()
		if containsCloseRef(body, issueNumber) {
			return prFromREST(pr), nil
		}
	}
	return nil, fmt.Errorf("no PR found referencing issue #%d", issueNumber)
}

func containsCloseRef(body string, issue int) bool {
	ref := fmt.Sprintf("#%d", issue)
	return len(body) > 0 && (contains(body, "Closes "+ref) || contains(body, "closes "+ref) ||
		contains(body, "Fixes "+ref) || contains(body, "fixes "+ref) ||
		contains(body, "Resolves "+ref) || contains(body, "resolves "+ref))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func prFromREST(pr *gh.PullRequest) *PullRequest {
	return &PullRequest{
		Number:       pr.GetNumber(),
		Title:        pr.GetTitle(),
		URL:          pr.GetHTMLURL(),
		State:        pr.GetState(),
		IsDraft:      pr.GetDraft(),
		HeadRefName:  pr.GetHead().GetRef(),
		BaseRefName:  pr.GetBase().GetRef(),
		Additions:    pr.GetAdditions(),
		Deletions:    pr.GetDeletions(),
		ChangedFiles: pr.GetChangedFiles(),
	}
}
