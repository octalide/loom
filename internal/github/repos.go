package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v84/github"
)

type RepoSettings struct {
	AutoDelete    bool
	AutoMerge     bool
	DefaultBranch string
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
		AutoDelete:    r.GetDeleteBranchOnMerge(),
		AutoMerge:     r.GetAllowAutoMerge(),
		DefaultBranch: r.GetDefaultBranch(),
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

func (c *Client) SetDefaultBranch(ctx context.Context, repo, branch string) error {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return err
	}
	_, _, err = c.REST.Repositories.Edit(ctx, owner, name, &gh.Repository{
		DefaultBranch: gh.Ptr(branch),
	})
	if err != nil {
		return fmt.Errorf("set default branch to %s: %w", branch, err)
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
		emptyContexts := []string{}
		req.RequiredStatusChecks = &gh.RequiredStatusChecks{
			Strict:   branch == "dev",
			Checks:   &checks,
			Contexts: &emptyContexts,
		}
	}

	_, _, err = c.REST.Repositories.UpdateBranchProtection(ctx, owner, name, branch, req)
	if err != nil {
		return fmt.Errorf("set branch protection on %s: %w", branch, err)
	}
	return nil
}

func (c *Client) GetBranchCIStatus(ctx context.Context, repo, branch string) (string, []CheckStatus, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return "", nil, err
	}
	ref := "heads/" + branch
	var checks []CheckStatus
	state := ""

	combined, _, err := c.REST.Repositories.GetCombinedStatus(ctx, owner, name, ref, nil)
	if err == nil && combined != nil {
		state = combined.GetState()
		for _, s := range combined.Statuses {
			checks = append(checks, CheckStatus{
				Name:       s.GetContext(),
				Conclusion: s.GetState(),
			})
		}
	}

	checkRuns, _, err2 := c.REST.Checks.ListCheckRunsForRef(ctx, owner, name, ref, &gh.ListCheckRunsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err2 == nil && checkRuns != nil {
		for _, cr := range checkRuns.CheckRuns {
			checks = append(checks, CheckStatus{
				Name:       cr.GetName(),
				Status:     cr.GetStatus(),
				Conclusion: cr.GetConclusion(),
			})
		}
	}

	if state == "" && len(checks) > 0 {
		state = "pending"
		allPass := true
		for _, c := range checks {
			if c.Conclusion == "failure" || c.Conclusion == "error" {
				state = "failure"
				allPass = false
				break
			}
			if c.Status != "completed" {
				allPass = false
			}
		}
		if allPass && state != "failure" {
			state = "success"
		}
	}
	return state, checks, nil
}

func (c *Client) CreateRelease(ctx context.Context, repo, tag, title, body string) (string, error) {
	owner, name, err := SplitRepo(repo)
	if err != nil {
		return "", err
	}
	release, _, err := c.REST.Repositories.CreateRelease(ctx, owner, name, &gh.RepositoryRelease{
		TagName: gh.Ptr(tag),
		Name:    gh.Ptr(title),
		Body:    gh.Ptr(body),
	})
	if err != nil {
		return "", fmt.Errorf("create release: %w", err)
	}
	return release.GetHTMLURL(), nil
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
