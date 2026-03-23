package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v84/github"
	"github.com/shurcooL/githubv4"
)

type PullRequest struct {
	Number       int
	Title        string
	Body         string
	URL          string
	State        string
	Merged       bool
	IsDraft      bool
	HeadRefName  string
	BaseRefName  string
	Additions    int
	Deletions    int
	ChangedFiles int
	CreatedAt    string
	UpdatedAt    string
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
	var result []*PullRequest
	opts := &gh.PullRequestListOptions{
		State:       "open",
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	for {
		prs, resp, err := c.REST.PullRequests.List(ctx, owner, name, opts)
		if err != nil {
			return nil, fmt.Errorf("list open PRs: %w", err)
		}
		for _, pr := range prs {
			result = append(result, prFromREST(pr))
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Client) ReadyPR(ctx context.Context, repo string, number int) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	pr, _, err := c.REST.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return fmt.Errorf("get PR #%d: %w", number, err)
	}
	if !pr.GetDraft() {
		return nil
	}
	var mutation struct {
		MarkPullRequestReadyForReview struct {
			ClientMutationID string
		} `graphql:"markPullRequestReadyForReview(input: $input)"`
	}
	if err := c.GraphQL.Mutate(ctx, &mutation, githubv4.MarkPullRequestReadyForReviewInput{
		PullRequestID: githubv4.ID(pr.GetNodeID()),
	}, nil); err != nil {
		return fmt.Errorf("mark PR #%d ready: %w", number, err)
	}
	return nil
}

// ReadyAndAutoMerge marks a PR as ready for review and enables auto-merge
// in a single flow using GraphQL for both operations to avoid REST→GraphQL
// race conditions.
func (c *Client) ReadyAndAutoMerge(ctx context.Context, repo string, number int, mergeMethod string) (readied bool, err error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return false, err
	}

	pr, _, err := c.REST.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return false, fmt.Errorf("get PR #%d: %w", number, err)
	}

	nodeID := pr.GetNodeID()
	wasDraft := pr.GetDraft()

	if wasDraft {
		var readyMutation struct {
			MarkPullRequestReadyForReview struct {
				ClientMutationID string
			} `graphql:"markPullRequestReadyForReview(input: $input)"`
		}
		if err := c.GraphQL.Mutate(ctx, &readyMutation, githubv4.MarkPullRequestReadyForReviewInput{
			PullRequestID: githubv4.ID(nodeID),
		}, nil); err != nil {
			return false, fmt.Errorf("mark PR #%d ready: %w", number, err)
		}
	}

	var mergeMutation struct {
		EnablePullRequestAutoMerge struct {
			ClientMutationID string
		} `graphql:"enablePullRequestAutoMerge(input: $input)"`
	}
	method := githubv4.PullRequestMergeMethod(mergeMethod)
	if err := c.GraphQL.Mutate(ctx, &mergeMutation, githubv4.EnablePullRequestAutoMergeInput{
		PullRequestID: githubv4.ID(nodeID),
		MergeMethod:   &method,
	}, nil); err != nil {
		return wasDraft, fmt.Errorf("enable auto-merge for PR #%d: %w", number, err)
	}

	return wasDraft, nil
}

func (c *Client) UpdatePRBranch(ctx context.Context, repo string, number int) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, _, err = c.REST.PullRequests.UpdateBranch(ctx, owner, name, number, nil)
	if err != nil {
		return fmt.Errorf("update PR #%d branch: %w", number, err)
	}
	return nil
}

func (c *Client) MergePR(ctx context.Context, repo string, number int, mergeMethod string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	method := "merge"
	switch strings.ToUpper(mergeMethod) {
	case "SQUASH":
		method = "squash"
	case "REBASE":
		method = "rebase"
	}
	_, _, err = c.REST.PullRequests.Merge(ctx, owner, name, number, "", &gh.PullRequestOptions{
		MergeMethod: method,
	})
	if err != nil {
		return fmt.Errorf("merge PR #%d: %w", number, err)
	}
	return nil
}

func (c *Client) UpdatePRBody(ctx context.Context, repo string, number int, body string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, _, err = c.REST.PullRequests.Edit(ctx, owner, name, number, &gh.PullRequest{
		Body: gh.Ptr(body),
	})
	if err != nil {
		return fmt.Errorf("update PR #%d body: %w", number, err)
	}
	return nil
}

func (c *Client) EnableAutoMerge(ctx context.Context, repo string, number int, mergeMethod string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	pr, _, err := c.REST.PullRequests.Get(ctx, owner, name, number)
	if err != nil {
		return fmt.Errorf("get PR #%d: %w", number, err)
	}
	nodeID := pr.GetNodeID()
	var mutation struct {
		EnablePullRequestAutoMerge struct {
			ClientMutationID string
		} `graphql:"enablePullRequestAutoMerge(input: $input)"`
	}
	method := githubv4.PullRequestMergeMethod(mergeMethod)
	if err := c.GraphQL.Mutate(ctx, &mutation, githubv4.EnablePullRequestAutoMergeInput{
		PullRequestID: githubv4.ID(nodeID),
		MergeMethod:   &method,
	}, nil); err != nil {
		return fmt.Errorf("enable auto-merge for PR #%d: %w", number, err)
	}
	return nil
}

func (c *Client) GetPRReviews(ctx context.Context, repo string, number int) ([]Review, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	var result []Review
	opts := &gh.ListOptions{PerPage: 100}
	for {
		reviews, resp, err := c.REST.PullRequests.ListReviews(ctx, owner, name, number, opts)
		if err != nil {
			return nil, fmt.Errorf("list reviews for PR #%d: %w", number, err)
		}
		for _, r := range reviews {
			result = append(result, Review{
				Author:      r.GetUser().GetLogin(),
				State:       r.GetState(),
				Body:        r.GetBody(),
				SubmittedAt: r.GetSubmittedAt().Format("2006-01-02T15:04:05Z"),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return result, nil
}

func (c *Client) GetPRReviewComments(ctx context.Context, repo string, number int) (map[string][]ReviewComment, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	byFile := make(map[string][]ReviewComment)
	opts := &gh.PullRequestListCommentsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	for {
		comments, resp, err := c.REST.PullRequests.ListComments(ctx, owner, name, number, opts)
		if err != nil {
			return nil, fmt.Errorf("list review comments for PR #%d: %w", number, err)
		}
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
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
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
	ref := fmt.Sprintf("#%d", issueNumber)
	query := fmt.Sprintf("repo:%s/%s is:pr %s in:body", owner, name, ref)
	results, _, err := c.REST.Search.Issues(ctx, query, &gh.SearchOptions{
		Sort:        "updated",
		Order:       "desc",
		ListOptions: gh.ListOptions{PerPage: 20},
	})
	if err != nil {
		return nil, fmt.Errorf("search PRs for issue #%d: %w", issueNumber, err)
	}
	for _, issue := range results.Issues {
		if !issue.IsPullRequest() {
			continue
		}
		body := issue.GetBody()
		if ContainsCloseRef(body, issueNumber) {
			pr, _, prErr := c.REST.PullRequests.Get(ctx, owner, name, issue.GetNumber())
			if prErr != nil {
				continue
			}
			return prFromREST(pr), nil
		}
	}
	return nil, fmt.Errorf("no PR found referencing issue #%d", issueNumber)
}

// ContainsCloseRef checks whether body contains a GitHub auto-close reference
// (Closes #N, Fixes #N, or Resolves #N) for the given issue number.
func ContainsCloseRef(body string, issue int) bool {
	ref := fmt.Sprintf("#%d", issue)
	lower := strings.ToLower(body)
	return strings.Contains(lower, "closes "+ref) ||
		strings.Contains(lower, "fixes "+ref) ||
		strings.Contains(lower, "resolves "+ref)
}

func prFromREST(pr *gh.PullRequest) *PullRequest {
	return &PullRequest{
		Number:       pr.GetNumber(),
		Title:        pr.GetTitle(),
		Body:         pr.GetBody(),
		URL:          pr.GetHTMLURL(),
		State:        pr.GetState(),
		Merged:       pr.GetMerged(),
		IsDraft:      pr.GetDraft(),
		HeadRefName:  pr.GetHead().GetRef(),
		BaseRefName:  pr.GetBase().GetRef(),
		Additions:    pr.GetAdditions(),
		Deletions:    pr.GetDeletions(),
		ChangedFiles: pr.GetChangedFiles(),
		CreatedAt:    pr.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    pr.GetUpdatedAt().Format("2006-01-02T15:04:05Z"),
	}
}
