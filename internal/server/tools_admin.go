package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/detect"
)

type linkInput struct {
	Issue        string `json:"issue" jsonschema:"Source issue number"`
	Target       string `json:"target" jsonschema:"Target issue number"`
	Relationship string `json:"relationship" jsonschema:"Relationship type: blocked_by blocks parent_of child_of. Prefix with - to remove."`
	TargetRepo   string `json:"target_repo,omitempty" jsonschema:"Target repo if different from source. Defaults to same repo."`
	Repo         string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd          string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleLink(ctx context.Context, req *mcp.CallToolRequest, in linkInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}
	targetRepo := detect.FirstNonEmpty(in.TargetRepo, repo)

	issueNum := parseInt(in.Issue)
	targetNum := parseInt(in.Target)

	relationship := in.Relationship
	remove := strings.HasPrefix(relationship, "-")
	if remove {
		relationship = relationship[1:]
	}

	valid := map[string]bool{"blocked_by": true, "blocks": true, "parent_of": true, "child_of": true}
	if !valid[relationship] {
		return errorResult("relationship must be one of blocked_by, blocks, parent_of, child_of; got %q", relationship), nil, nil
	}

	var err error
	if relationship == "blocked_by" || relationship == "blocks" {
		if remove {
			err = s.gh.RemoveBlockingLink(ctx, repo, issueNum, targetRepo, targetNum, relationship)
		} else {
			err = s.gh.AddBlockingLink(ctx, repo, issueNum, targetRepo, targetNum, relationship)
		}
	} else {
		if remove {
			err = s.gh.RemoveSubIssueLink(ctx, repo, issueNum, targetRepo, targetNum, relationship)
		} else {
			err = s.gh.AddSubIssueLink(ctx, repo, issueNum, targetRepo, targetNum, relationship)
		}
	}

	if err != nil {
		action := "add"
		if remove {
			action = "remove"
		}
		return errorResult("failed to %s %s: %v", action, relationship, err), nil, nil
	}

	action := "Added"
	if remove {
		action = "Removed"
	}
	cross := ""
	if targetRepo != repo {
		cross = fmt.Sprintf(" (%s)", targetRepo)
	}

	b := newBuilder()
	b.OK("%s %s: #%d → #%d%s", action, relationship, issueNum, targetNum, cross)
	return builderResult(b), nil, nil
}

type boardStatusInput struct {
	Issue   string `json:"issue" jsonschema:"Issue number"`
	Status  string `json:"status" jsonschema:"Target status: Todo or In Progress or Done"`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Project string `json:"project,omitempty" jsonschema:"Project number. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleBoardStatus(ctx context.Context, req *mcp.CallToolRequest, in boardStatusInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}
	project := detect.FirstNonZero(parseInt(in.Project), dc.Project)
	if project == 0 {
		return errorResult("could not detect project number; pass explicitly or add to .github/loom.yml"), nil, nil
	}

	issueNum := parseInt(in.Issue)

	cfg := dc.Config
	validStatuses := []string{cfg.Statuses.Todo, cfg.Statuses.InProgress, cfg.Statuses.Done}
	found := false
	for _, v := range validStatuses {
		if v == in.Status {
			found = true
			break
		}
	}
	if !found {
		return errorResult("status must be one of %s; got %q", strings.Join(validStatuses, ", "), in.Status), nil, nil
	}

	owner := detect.FirstNonEmpty(dc.Owner, detect.OwnerOf(repo))

	issueURL, err := s.gh.GetIssueURL(ctx, repo, issueNum)
	if err != nil {
		return errorResult("could not find issue #%d: %v", issueNum, err), nil, nil
	}

	if err := s.gh.SetProjectStatus(ctx, owner, project, issueURL, in.Status, ""); err != nil {
		return errorResult("failed to set status: %v", err), nil, nil
	}

	b := newBuilder()
	b.OK("Issue #%d → '%s' on project #%d", issueNum, in.Status, project)

	if in.Status == cfg.Statuses.Done {
		if err := s.gh.ArchiveProjectItem(ctx, owner, project, issueURL); err != nil {
			b.Warn("archive failed: %v", err)
		} else {
			b.OK("Archived on project board")
		}
	}

	return builderResult(b), nil, nil
}

type auditInput struct {
	Fix     bool   `json:"fix,omitempty" jsonschema:"Auto-fix safe issues. Default: false"`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Project string `json:"project,omitempty" jsonschema:"Project number. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleAudit(ctx context.Context, req *mcp.CallToolRequest, in auditInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}
	project := detect.FirstNonZero(parseInt(in.Project), dc.Project)
	cfg := dc.Config

	b := newBuilder()
	b.Header(fmt.Sprintf("Audit: %s (project #%d)", repo, project))

	var fixed []string

	// Auth
	hasProject, err := s.gh.CheckAuth(s.gh.Token())
	_ = hasProject
	if err != nil {
		b.Warn("gh auth: not authenticated")
	} else {
		b.OK("gh auth: authenticated")
	}

	// Repo settings
	settings, err := s.gh.GetRepoSettings(ctx, repo)
	if err == nil {
		if !settings.AutoDelete {
			b.Warn("Repo: auto-delete head branches is disabled")
			if in.Fix {
				if err := s.gh.SetRepoSettings(ctx, repo, true, settings.AutoMerge); err == nil {
					fixed = append(fixed, "Enabled auto-delete head branches")
				}
			}
		} else {
			b.OK("Repo: auto-delete head branches enabled")
		}
		if !settings.AutoMerge {
			b.Warn("Repo: auto-merge is disabled")
			if in.Fix {
				if err := s.gh.SetRepoSettings(ctx, repo, settings.AutoDelete, true); err == nil {
					fixed = append(fixed, "Enabled auto-merge")
				}
			}
		} else {
			b.OK("Repo: auto-merge enabled")
		}
	}

	// Branch protection
	for _, branch := range []string{cfg.Branches.Base, cfg.Branches.Release} {
		bp, err := s.gh.GetBranchProtection(ctx, repo, branch)
		if err != nil {
			b.Warn("Branch '%s': could not check protection: %v", branch, err)
			continue
		}
		if !bp.Protected {
			b.Warn("Branch '%s': no protection rules", branch)
			if in.Fix {
				if err := s.gh.SetBranchProtection(ctx, repo, branch, nil); err == nil {
					fixed = append(fixed, fmt.Sprintf("Set branch protection on '%s'", branch))
				}
			}
		} else {
			var issues []string
			if !bp.RequirePR {
				issues = append(issues, "PRs not required")
			}
			if bp.AllowDeletions {
				issues = append(issues, "deletion allowed")
			}
			if len(issues) > 0 {
				b.Warn("Branch '%s': %s", branch, strings.Join(issues, ", "))
				if in.Fix {
					if err := s.gh.SetBranchProtection(ctx, repo, branch, bp.StatusChecks); err == nil {
						fixed = append(fixed, fmt.Sprintf("Fixed branch protection on '%s'", branch))
					}
				}
			} else {
				b.OK("Branch '%s': protected", branch)
			}
		}
	}

	// CI workflows
	if s.gh.WorkflowExists(ctx, repo, "ci.yml") {
		b.OK("CI workflow: ci.yml exists")
	} else {
		b.Warn("CI workflow: no .github/workflows/ci.yml found")
	}
	if s.gh.WorkflowExists(ctx, repo, "release.yml") {
		b.OK("Release workflow: release.yml exists")
	} else {
		b.Warn("Release workflow: no .github/workflows/release.yml found")
	}

	// Current branch
	branch := dc.BranchName
	if branch == "" {
		b.Warn("Could not determine current branch")
		return builderResult(b), nil, nil
	}
	isFeature := branch != cfg.Branches.Base && branch != cfg.Branches.Release
	b.Info("Current branch: %s", branch)

	// Uncommitted changes
	if s.git.HasUncommittedChanges(dc.Cwd) {
		b.Warn("Uncommitted changes detected")
	} else {
		b.OK("Working tree clean")
	}

	// Stale merged branches
	stale, _ := s.git.MergedBranches(dc.Cwd, cfg.Branches.Base, true)
	if len(stale) > 0 {
		b.Warn("Stale merged branches (%d): %s", len(stale), strings.Join(stale, ", "))
		if in.Fix {
			for _, br := range stale {
				if err := s.git.DeleteLocalBranch(dc.Cwd, br); err == nil {
					fixed = append(fixed, fmt.Sprintf("Deleted stale branch '%s'", br))
				}
			}
		}
	} else {
		b.OK("No stale merged branches")
	}

	// Missing PR for feature branch
	if isFeature {
		if _, err := s.gh.FindPRForBranch(ctx, repo, branch); err != nil {
			b.Warn("No PR found for branch '%s'", branch)
			if in.Fix {
				m := regexp.MustCompile(`/(\d+)$`).FindStringSubmatch(branch)
				issueNum := 0
				if m != nil {
					fmt.Sscanf(m[1], "%d", &issueNum)
				}
				prBody := ""
				if issueNum > 0 {
					prBody = fmt.Sprintf("Closes #%d", issueNum)
				}
				prTitle := strings.Replace(branch, "/", ": ", 1)
				if len(prTitle) > 70 {
					prTitle = prTitle[:67] + "..."
				}
				if _, url, err := s.gh.CreateDraftPR(ctx, repo, prTitle, prBody, cfg.Branches.Base, branch); err == nil {
					fixed = append(fixed, fmt.Sprintf("Created draft PR: %s", url))
				}
			}
		} else {
			b.OK("PR exists for '%s'", branch)
		}
	}

	// Dev behind remote
	if s.git.IsBehindRemote(dc.Cwd, cfg.Branches.Base) {
		b.Warn("Local '%s' is behind origin/%s", cfg.Branches.Base, cfg.Branches.Base)
		if in.Fix && !isFeature {
			if err := s.git.CheckoutAndPull(dc.Cwd, cfg.Branches.Base); err == nil {
				fixed = append(fixed, fmt.Sprintf("Synced %s with origin/%s", cfg.Branches.Base, cfg.Branches.Base))
			}
		}
	} else {
		b.OK("Local '%s' is up to date with remote", cfg.Branches.Base)
	}

	// Stale worktrees
	_ = s.git.PruneRemotes(dc.Cwd)
	worktrees, _ := s.git.ListWorktrees(dc.Cwd)
	if len(worktrees) > 1 {
		merged := make(map[string]bool)
		mergedBranches, _ := s.git.MergedBranches(dc.Cwd, cfg.Branches.Base, true)
		for _, br := range mergedBranches {
			merged[br] = true
		}
		var staleWTs []string
		for _, wt := range worktrees[1:] {
			if wt.Branch == "" {
				continue
			}
			if merged[wt.Branch] || !s.git.RemoteBranchExists(dc.Cwd, wt.Branch) {
				staleWTs = append(staleWTs, fmt.Sprintf("%s (%s)", wt.Branch, wt.Path))
				if in.Fix {
					if err := s.git.RemoveWorktree(dc.Cwd, wt.Path); err == nil {
						fixed = append(fixed, fmt.Sprintf("Removed stale worktree: %s", wt.Path))
						_ = s.git.DeleteLocalBranch(dc.Cwd, wt.Branch)
					}
				}
			}
		}
		if len(staleWTs) > 0 {
			b.Warn("Stale worktrees (%d): %s", len(staleWTs), strings.Join(staleWTs, ", "))
		} else {
			b.OK("%d active worktree(s), none stale", len(worktrees)-1)
		}
	} else {
		b.OK("No linked worktrees")
	}

	if len(fixed) > 0 {
		b.Section(fmt.Sprintf("Fixed (%d)", len(fixed)))
		for _, f := range fixed {
			b.Bullet(f)
		}
	} else if in.Fix {
		b.Section("Fixed")
		b.Text("Nothing to fix.")
	}

	return builderResult(b), nil, nil
}

type setupInput struct {
	Repo string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd  string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleSetup(ctx context.Context, req *mcp.CallToolRequest, in setupInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}
	cfg := dc.Config

	b := newBuilder()
	b.Header(fmt.Sprintf("Setup: %s", repo))

	checks := cfg.Checks

	// Repo settings
	if err := s.gh.SetRepoSettings(ctx, repo, true, true); err != nil {
		b.Warn("Repo settings failed: %v", err)
	} else {
		b.OK("Repo: auto-delete branches on merge, auto-merge enabled")
	}

	// Ensure base branch exists
	if !s.git.RemoteBranchExists(dc.Cwd, cfg.Branches.Base) {
		b.Info("Creating %s branch from %s...", cfg.Branches.Base, cfg.Branches.Release)
		_ = s.git.Checkout(dc.Cwd, cfg.Branches.Release)
		_ = s.git.CreateBranch(dc.Cwd, cfg.Branches.Base, cfg.Branches.Release)
		_ = s.git.Push(dc.Cwd, cfg.Branches.Base)
		if dc.BranchName != "" && dc.BranchName != cfg.Branches.Release && dc.BranchName != cfg.Branches.Base {
			_ = s.git.Checkout(dc.Cwd, dc.BranchName)
		}
	}
	b.OK("Branch: %s exists", cfg.Branches.Base)

	// Branch protection
	for _, branch := range []string{cfg.Branches.Base, cfg.Branches.Release} {
		if err := s.gh.SetBranchProtection(ctx, repo, branch, checks); err != nil {
			b.Warn("Protection %s failed: %v", branch, err)
		} else {
			checkInfo := " (no required checks)"
			if len(checks) > 0 {
				checkInfo = fmt.Sprintf(" (checks: %s)", strings.Join(checks, ", "))
			}
			b.OK("Protection: %s — PR required, no deletion%s", branch, checkInfo)
		}
	}

	if len(checks) == 0 {
		b.Text("")
		b.Text("No CI checks configured. Add a 'checks' list to .github/loom.yml to require status checks on PRs.")
	}

	return builderResult(b), nil, nil
}
