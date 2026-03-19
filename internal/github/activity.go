package github

import (
	"context"
	"time"

	gh "github.com/google/go-github/v84/github"
)

type Activity struct {
	IssueComments    []IssueComment
	LinkedPR         *PullRequest
	PRReviews        []Review
	PRReviewComments map[string][]ReviewComment
	PRComments       []IssueComment
	Checks           []CheckStatus
}

func (c *Client) GetActivity(ctx context.Context, repo string, issueNumber int, since string) (*Activity, error) {
	var sinceTime time.Time
	if since != "" {
		var err error
		sinceTime, err = time.Parse(time.RFC3339, since)
		if err != nil {
			sinceTime = time.Time{}
		}
	}

	result := &Activity{
		PRReviewComments: make(map[string][]ReviewComment),
	}

	// Issue comments
	comments, err := c.GetIssueComments(ctx, repo, issueNumber)
	if err == nil {
		for _, c := range comments {
			if !sinceTime.IsZero() {
				t, _ := time.Parse(time.RFC3339, c.CreatedAt)
				if t.Before(sinceTime) {
					continue
				}
			}
			result.IssueComments = append(result.IssueComments, c)
		}
	}

	// Find linked PR
	pr, err := c.FindPRForIssue(ctx, repo, issueNumber)
	if err != nil {
		return result, nil
	}
	result.LinkedPR = pr

	// PR reviews
	reviews, err := c.GetPRReviews(ctx, repo, pr.Number)
	if err == nil {
		for _, r := range reviews {
			if !sinceTime.IsZero() {
				t, _ := time.Parse(time.RFC3339, r.SubmittedAt)
				if t.Before(sinceTime) {
					continue
				}
			}
			result.PRReviews = append(result.PRReviews, r)
		}
	}

	// PR inline comments
	byFile, err := c.GetPRReviewComments(ctx, repo, pr.Number)
	if err == nil {
		for path, comments := range byFile {
			for _, c := range comments {
				if !sinceTime.IsZero() {
					t, _ := time.Parse(time.RFC3339, c.CreatedAt)
					if t.Before(sinceTime) {
						continue
					}
				}
				result.PRReviewComments[path] = append(result.PRReviewComments[path], c)
			}
		}
	}

	// PR conversation comments
	prComments, err := c.GetPRComments(ctx, repo, pr.Number)
	if err == nil {
		for _, c := range prComments {
			if !sinceTime.IsZero() {
				t, _ := time.Parse(time.RFC3339, c.CreatedAt)
				if t.Before(sinceTime) {
					continue
				}
			}
			result.PRComments = append(result.PRComments, c)
		}
	}

	return result, nil
}

// AssessMergeReadiness checks PR merge readiness via GraphQL for full detail.
func (c *Client) AssessMergeReadiness(ctx context.Context, repo string, prNumber int) (*MergeReadiness, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}

	pr, _, err := c.REST.PullRequests.Get(ctx, owner, name, prNumber)
	if err != nil {
		return nil, err
	}

	readiness := &MergeReadiness{
		IsDraft:    pr.GetDraft(),
		Mergeable:  pr.GetMergeableState(),
		MergeState: pr.GetMergeableState(),
	}

	if pr.AutoMerge != nil {
		readiness.AutoMerge = true
		readiness.AutoMergeMethod = pr.GetAutoMerge().GetMergeMethod()
	}

	// Build summary
	if readiness.AutoMerge {
		readiness.Summary = append(readiness.Summary, "Auto-merge: ENABLED ("+readiness.AutoMergeMethod+")")
	} else {
		readiness.Summary = append(readiness.Summary, "Auto-merge: not configured")
	}

	if readiness.IsDraft {
		readiness.Summary = append(readiness.Summary, "PR state: DRAFT")
	} else {
		readiness.Summary = append(readiness.Summary, "PR state: ready for review")
	}

	switch readiness.MergeState {
	case "clean":
		readiness.Summary = append(readiness.Summary, "Merge state: CLEAN")
	case "blocked":
		readiness.Summary = append(readiness.Summary, "Merge state: BLOCKED")
	case "behind":
		readiness.Summary = append(readiness.Summary, "Merge state: BEHIND — update needed")
	case "dirty":
		readiness.Summary = append(readiness.Summary, "Merge state: DIRTY — conflicts present")
	case "draft":
		readiness.Summary = append(readiness.Summary, "Merge state: DRAFT")
	case "unstable":
		readiness.Summary = append(readiness.Summary, "Merge state: UNSTABLE — checks not passing")
	default:
		readiness.Summary = append(readiness.Summary, "Merge state: "+readiness.MergeState)
	}

	// Get combined status
	statuses, _, err := c.REST.Repositories.GetCombinedStatus(ctx, owner, name, pr.GetHead().GetSHA(), nil)
	if err == nil {
		for _, s := range statuses.Statuses {
			readiness.Checks = append(readiness.Checks, CheckStatus{
				Name:       s.GetContext(),
				Conclusion: s.GetState(),
			})
		}
	}

	// Get check runs
	checkRuns, _, err := c.REST.Checks.ListCheckRunsForRef(ctx, owner, name, pr.GetHead().GetSHA(), &gh.ListCheckRunsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err == nil {
		for _, cr := range checkRuns.CheckRuns {
			readiness.Checks = append(readiness.Checks, CheckStatus{
				Name:       cr.GetName(),
				Status:     cr.GetStatus(),
				Conclusion: cr.GetConclusion(),
			})
		}
	}

	return readiness, nil
}
