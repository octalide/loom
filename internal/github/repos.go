package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v84/github"
)

type RepoSettings struct {
	AutoDelete bool
	AutoMerge  bool
}

type BranchProtection struct {
	Protected          bool
	RequirePR          bool
	RequireStatusChecks bool
	StatusChecks       []string
	AllowDeletions     bool
	EnforceAdmins      bool
}

func (c *Client) GetRepoSettings(ctx context.Context, repo string) (*RepoSettings, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	r, _, err := c.REST.Repositories.Get(ctx, owner, name)
	if err != nil {
		return nil, fmt.Errorf("get repo settings: %w", err)
	}
	return &RepoSettings{
		AutoDelete: r.GetDeleteBranchOnMerge(),
		AutoMerge:  r.GetAllowAutoMerge(),
	}, nil
}

func (c *Client) SetRepoSettings(ctx context.Context, repo string, autoDelete, autoMerge bool) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, _, err = c.REST.Repositories.Edit(ctx, owner, name, &gh.Repository{
		DeleteBranchOnMerge: gh.Ptr(autoDelete),
		AllowAutoMerge:      gh.Ptr(autoMerge),
	})
	if err != nil {
		return fmt.Errorf("set repo settings: %w", err)
	}
	return nil
}

func (c *Client) GetBranchProtection(ctx context.Context, repo, branch string) (*BranchProtection, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return nil, err
	}
	bp, resp, err := c.REST.Repositories.GetBranchProtection(ctx, owner, name, branch)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return &BranchProtection{Protected: false}, nil
		}
		return nil, fmt.Errorf("get branch protection: %w", err)
	}

	result := &BranchProtection{
		Protected: true,
	}
	if ad := bp.GetAllowDeletions(); ad != nil {
		result.AllowDeletions = ad.Enabled
	}
	if ea := bp.GetEnforceAdmins(); ea != nil {
		result.EnforceAdmins = ea.Enabled
	}
	if bp.RequiredPullRequestReviews != nil {
		result.RequirePR = true
	}
	if bp.RequiredStatusChecks != nil && bp.RequiredStatusChecks.Contexts != nil {
		result.StatusChecks = *bp.RequiredStatusChecks.Contexts
		result.RequireStatusChecks = len(result.StatusChecks) > 0
	}
	return result, nil
}

func (c *Client) SetBranchProtection(ctx context.Context, repo, branch string, statusChecks []string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}

	req := &gh.ProtectionRequest{
		RequiredPullRequestReviews: &gh.PullRequestReviewsEnforcementRequest{
			RequiredApprovingReviewCount: 0,
		},
		EnforceAdmins: false,
	}

	if len(statusChecks) > 0 {
		checks := make([]*gh.RequiredStatusCheck, len(statusChecks))
		for i, check := range statusChecks {
			checks[i] = &gh.RequiredStatusCheck{Context: check}
		}
		req.RequiredStatusChecks = &gh.RequiredStatusChecks{
			Strict: branch == "dev",
			Checks: &checks,
		}
	}

	_, _, err = c.REST.Repositories.UpdateBranchProtection(ctx, owner, name, branch, req)
	if err != nil {
		return fmt.Errorf("set branch protection on %s: %w", branch, err)
	}
	return nil
}

func (c *Client) WorkflowExists(ctx context.Context, repo, workflowName string) bool {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return false
	}
	_, _, resp, err := c.REST.Repositories.GetContents(ctx, owner, name, ".github/workflows/"+workflowName, nil)
	if err != nil {
		return false
	}
	return resp.StatusCode == 200
}
