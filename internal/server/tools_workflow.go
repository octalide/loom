package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/detect"
	"github.com/octalide/loom/internal/output"
)

type createProjectInput struct {
	Title string `json:"title" jsonschema:"Project title"`
	Owner string `json:"owner,omitempty" jsonschema:"GitHub owner (user or org). Auto-detected if omitted."`
}

func (s *Server) handleCreateProject(ctx context.Context, req *mcp.CallToolRequest, in createProjectInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	owner := in.Owner
	if owner == "" {
		var err error
		owner, err = s.gh.AuthenticatedUser(ctx)
		if err != nil {
			return errorResult("could not determine owner: %v", err), nil, nil
		}
	}

	number, url, err := s.gh.CreateProject(ctx, in.Title, owner)
	if err != nil {
		return errorResult("failed to create project: %v", err), nil, nil
	}

	b := newBuilder()
	b.Header("Project Created")
	b.KV("Number", fmt.Sprintf("#%d", number))
	b.KV("URL", url)
	return builderResult(b), nil, nil
}

type createIssueInput struct {
	Title   string `json:"title" jsonschema:"Issue title"`
	Body    string `json:"body" jsonschema:"Issue body (markdown)"`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository in owner/repo format. Auto-detected if omitted."`
	Project int    `json:"project,omitempty" jsonschema:"GitHub Project number. Auto-detected from .github/loom.yml if omitted."`
	Labels  string `json:"labels,omitempty" jsonschema:"Comma-separated label names to apply"`
}

func (s *Server) handleCreateIssue(ctx context.Context, req *mcp.CallToolRequest, in createIssueInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect("")
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}
	project := detect.FirstNonZero(in.Project, dc.Project)

	b := newBuilder()

	// Create issue
	number, url, err := s.gh.CreateIssue(ctx, repo, in.Title, in.Body)
	if err != nil {
		return errorResult("failed to create issue: %v", err), nil, nil
	}
	b.Header(fmt.Sprintf("Issue #%d", number))
	b.KV("URL", url)

	// Labels
	if in.Labels != "" {
		labels := parseCSV(in.Labels)
		if len(labels) > 0 {
			if err := s.gh.AddLabels(ctx, repo, number, labels); err != nil {
				b.Warn("failed to add labels: %v", err)
			} else {
				b.OK("Labels: %s", strings.Join(labels, ", "))
			}
		}
	}

	// Add to project board
	if project > 0 {
		owner := dc.Owner
		issueURL, _ := s.gh.GetIssueURL(ctx, repo, number)
		itemID, err := s.gh.AddIssueToProject(ctx, owner, project, issueURL)
		if err != nil {
			b.Warn("failed to add to project #%d: %v", project, err)
		} else {
			b.OK("Added to project #%d", project)

			// Set status to Todo
			status := dc.Config.Statuses.Todo
			if err := s.gh.SetProjectStatus(ctx, owner, project, issueURL, status, itemID); err != nil {
				b.Warn("failed to set status to %s: %v", status, err)
			} else {
				b.OK("Status → %s", status)
			}
		}
	}

	return builderResult(b), nil, nil
}

type startInput struct {
	Issue      int    `json:"issue" jsonschema:"Issue number to start working on"`
	Repo       string `json:"repo,omitempty" jsonschema:"Repository in owner/repo format. Auto-detected if omitted."`
	Project    int    `json:"project,omitempty" jsonschema:"GitHub Project number. Auto-detected if omitted."`
	BranchType string `json:"branch_type,omitempty" jsonschema:"Branch prefix: feat fix doc refactor issue. Default: feat"`
	Worktree   bool   `json:"worktree,omitempty" jsonschema:"Create a worktree instead of switching branches"`
	Cwd        string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleStart(ctx context.Context, req *mcp.CallToolRequest, in startInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}
	project := detect.FirstNonZero(in.Project, dc.Project)
	cfg := dc.Config

	branchType := in.BranchType
	if branchType == "" {
		branchType = "feat"
	}
	if !cfg.ValidBranchType(branchType) {
		return errorResult("invalid branch_type %q; allowed: %s", branchType, strings.Join(cfg.Branches.Types, ", ")), nil, nil
	}

	if !in.Worktree && s.git.HasUncommittedChanges(dc.Cwd) {
		return errorResult("uncommitted changes detected; commit or stash before starting a new issue"), nil, nil
	}

	// Fetch issue details
	issue, err := s.gh.GetIssue(ctx, repo, in.Issue)
	if err != nil {
		return errorResult("could not fetch issue #%d: %v", in.Issue, err), nil, nil
	}

	branchName := fmt.Sprintf("%s/%d", branchType, in.Issue)
	b := newBuilder()
	b.Header(fmt.Sprintf("Issue #%d: %s", in.Issue, issue.Title))

	var workCwd string
	branchExisted := false

	if in.Worktree {
		b.Info("Mode: worktree")

		if err := s.git.Fetch(dc.Cwd, "origin", cfg.Branches.Base); err != nil {
			return errorResult("failed to fetch origin/%s: %v", cfg.Branches.Base, err), nil, nil
		}
		b.OK("Fetched latest origin/%s", cfg.Branches.Base)

		wtPath, err := s.git.WorktreePathForBranch(dc.Cwd, branchName)
		if err != nil {
			return errorResult("could not compute worktree path: %v", err), nil, nil
		}

		if s.git.WorktreeExists(wtPath) {
			existing, _ := s.git.CurrentBranch(wtPath)
			if existing == branchName {
				b.OK("Resumed existing worktree: %s", wtPath)
				branchExisted = true
			} else {
				return errorResult("worktree path %q exists but is on branch %q, expected %q", wtPath, existing, branchName), nil, nil
			}
		} else {
			if err := s.git.CreateWorktree(dc.Cwd, wtPath, branchName, "origin/"+cfg.Branches.Base); err != nil {
				if err2 := s.git.AddWorktree(dc.Cwd, wtPath, branchName); err2 != nil {
					return errorResult("failed to create worktree: %v", err), nil, nil
				}
				b.OK("Created worktree (existing branch): %s", wtPath)
				branchExisted = true
			} else {
				b.OK("Created worktree: %s", wtPath)
			}
		}
		workCwd = wtPath
	} else {
		if err := s.git.CheckoutAndPull(dc.Cwd, cfg.Branches.Base); err != nil {
			return errorResult("failed to checkout/pull %s: %v", cfg.Branches.Base, err), nil, nil
		}
		b.OK("Checked out %s and pulled latest", cfg.Branches.Base)

		if err := s.git.CreateBranch(dc.Cwd, branchName, cfg.Branches.Base); err != nil {
			if err2 := s.git.Checkout(dc.Cwd, branchName); err2 != nil {
				return errorResult("failed to create branch %q: %v", branchName, err), nil, nil
			}
			b.OK("Resumed existing branch: %s", branchName)
			branchExisted = true
		} else {
			b.OK("Created branch: %s", branchName)
		}
		workCwd = dc.Cwd
	}

	// Push branch
	if err := s.git.Push(workCwd, branchName); err != nil {
		b.Warn("push failed (may already be up to date): %v", err)
	} else {
		b.OK("Pushed branch to origin")
	}

	// Set project status to In Progress
	if project > 0 && issue.URL != "" {
		status := cfg.Statuses.InProgress
		if err := s.gh.SetProjectStatus(ctx, dc.Owner, project, issue.URL, status, ""); err != nil {
			b.Warn("failed to set status to %s: %v", status, err)
		} else {
			b.OK("Status → %s", status)
		}
	}

	// Check for existing PR on resumed branches
	prURL := ""
	if branchExisted {
		if pr, err := s.gh.FindPRForBranch(ctx, repo, branchName); err == nil {
			prURL = pr.URL
		}
	}

	b.Section("Ready")
	b.KV("Branch", branchName)
	if prURL != "" {
		b.KV("PR", prURL)
	} else {
		b.Info("Draft PR will be created on first commit")
	}
	if in.Worktree {
		b.KV("Worktree", workCwd)
		b.Text("Use this path as cwd for commit and finish.")
	}

	return builderResult(b), nil, nil
}

type commitInput struct {
	Message string `json:"message" jsonschema:"Commit message following convention (e.g. feat: add login)"`
	Files   string `json:"files,omitempty" jsonschema:"Space-separated file paths to stage. Default: all changes"`
	Push    *bool  `json:"push,omitempty" jsonschema:"Push to remote after committing. Default: true"`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleCommit(ctx context.Context, req *mcp.CallToolRequest, in commitInput) (*mcp.CallToolResult, any, error) {
	dc := s.detect(in.Cwd)
	cfg := dc.Config

	branch := dc.BranchName
	if branch == "" {
		return errorResult("could not determine current branch"), nil, nil
	}
	if branch == cfg.Branches.Base || branch == cfg.Branches.Release {
		return errorResult("refusing to commit directly on '%s' — use a feature branch", branch), nil, nil
	}

	push := true
	if in.Push != nil {
		push = *in.Push
	}

	// Stage files
	var files []string
	if in.Files != "" && in.Files != "." {
		files = strings.Fields(in.Files)
	}
	if err := s.git.Stage(dc.Cwd, files); err != nil {
		return errorResult("failed to stage files: %v", err), nil, nil
	}

	if !s.git.HasStagedChanges(dc.Cwd) {
		return infoResult("Nothing to commit — no changes detected after staging"), nil, nil
	}

	if err := s.git.Commit(dc.Cwd, in.Message); err != nil {
		return errorResult("failed to commit: %v", err), nil, nil
	}

	b := newBuilder()
	b.Header("Committed")
	b.KV("Message", in.Message)

	if push {
		if err := s.git.Push(dc.Cwd, branch); err != nil {
			b.Warn("push failed: %v", err)
		} else {
			b.KV("Pushed", "origin/"+branch)
			s.ensureDraftPR(ctx, dc, branch, b)
		}
	}

	return builderResult(b), nil, nil
}

type finishInput struct {
	Issue   int    `json:"issue,omitempty" jsonschema:"Issue number. Auto-detected from branch name if omitted."`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Project int    `json:"project,omitempty" jsonschema:"Project number. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleFinish(ctx context.Context, req *mcp.CallToolRequest, in finishInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	issueNumber := detect.FirstNonZero(in.Issue, dc.IssueNumber)
	if issueNumber == 0 {
		return errorResult("could not detect issue number from branch %q; pass explicitly", dc.BranchName), nil, nil
	}

	branch := dc.BranchName
	if branch == "" {
		return errorResult("could not determine current branch"), nil, nil
	}

	expected := fmt.Sprintf("/%d", issueNumber)
	if !strings.HasSuffix(branch, expected) {
		return errorResult("current branch %q does not match issue #%d", branch, issueNumber), nil, nil
	}

	if s.git.HasUncommittedChanges(dc.Cwd) {
		return errorResult("uncommitted changes detected; commit or stash before finishing"), nil, nil
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("Finishing #%d", issueNumber))
	b.KV("Branch", branch)

	// Push remaining commits
	if err := s.git.Push(dc.Cwd, branch); err != nil {
		b.Warn("push failed (may be up to date): %v", err)
	} else {
		b.OK("Pushed latest commits")
	}

	// Find PR
	pr, err := s.gh.FindPRForBranch(ctx, repo, branch)
	if err != nil {
		return errorResult("no PR found for branch %q: %v", branch, err), nil, nil
	}
	b.OK("Found PR #%d", pr.Number)

	// Merge readiness
	readiness, err := s.gh.AssessMergeReadiness(ctx, repo, pr.Number)
	if err == nil {
		b.Section("Merge Readiness")
		for _, line := range readiness.Summary {
			b.Bullet(line)
		}
	}

	// Mark PR ready
	if err := s.gh.ReadyPR(ctx, repo, pr.Number); err != nil {
		b.Warn("failed to mark PR as ready: %v", err)
	} else {
		if pr.IsDraft {
			b.OK("Marked PR as ready")
		} else {
			b.OK("PR confirmed ready")
		}
	}

	// Enable auto-merge
	if err := s.gh.EnableAutoMerge(ctx, repo, pr.Number); err != nil {
		b.Warn("failed to enable auto-merge: %v", err)
	} else {
		b.OK("Auto-merge enabled (squash) for PR #%d", pr.Number)
	}

	// Cleanup
	b.Section("Cleanup")
	if dc.IsWorktree {
		wtPath, _ := s.git.Root(dc.Cwd)
		mainPath := dc.MainWorktree

		if err := s.git.RemoveWorktree(mainPath, wtPath); err != nil {
			b.Warn("failed to remove worktree: %v", err)
		} else {
			b.OK("Removed worktree: %s", wtPath)
		}
		if err := s.git.DeleteLocalBranch(mainPath, branch); err != nil {
			b.Warn("failed to delete local branch: %v", err)
		} else {
			b.OK("Deleted local branch '%s'", branch)
		}
		_ = s.git.PruneRemotes(mainPath)
		b.Section("Done")
		b.KV("Main repo", mainPath)
		b.Text("Worktree removed. Continue from the main repo.")
	} else {
		cfg := dc.Config
		if err := s.git.CheckoutAndPull(dc.Cwd, cfg.Branches.Base); err != nil {
			b.Warn("failed to checkout %s: %v", cfg.Branches.Base, err)
		} else {
			b.OK("Checked out %s and pulled", cfg.Branches.Base)
		}
		if err := s.git.DeleteLocalBranch(dc.Cwd, branch); err != nil {
			b.Warn("failed to delete local branch: %v", err)
		} else {
			b.OK("Deleted local branch '%s'", branch)
		}
		_ = s.git.PruneRemotes(dc.Cwd)
		b.Section("Done")
		b.Text("Ready for the next issue.")
	}

	b.Text("")
	b.Text("Auto-merge enabled. GitHub will merge the PR when CI passes and reviews are approved, then automatically close the issue and update the project board.")

	return builderResult(b), nil, nil
}

// ensureDraftPR creates a draft PR if one doesn't exist for the branch.
func (s *Server) ensureDraftPR(ctx context.Context, dc *detect.Context, branch string, b *output.Builder) {
	if s.gh == nil || dc.Repo == "" {
		return
	}

	// Check if PR already exists
	if _, err := s.gh.FindPRForBranch(ctx, dc.Repo, branch); err == nil {
		return
	}

	m := regexp.MustCompile(`^(\w+)/(\d+)$`).FindStringSubmatch(branch)
	if m == nil {
		return
	}

	branchType := m[1]
	issueNumber := 0
	fmt.Sscanf(m[2], "%d", &issueNumber)
	if issueNumber == 0 {
		return
	}

	issue, err := s.gh.GetIssue(ctx, dc.Repo, issueNumber)
	title := fmt.Sprintf("Issue #%d", issueNumber)
	if err == nil {
		title = issue.Title
	}

	prTitle := fmt.Sprintf("%s: %s", branchType, title)
	if len(prTitle) > 70 {
		prTitle = prTitle[:67] + "..."
	}

	_, prURL, err := s.gh.CreateDraftPR(ctx, dc.Repo, prTitle, fmt.Sprintf("Closes #%d", issueNumber), dc.Config.Branches.Base, branch)
	if err != nil {
		b.Warn("failed to create draft PR: %v", err)
		return
	}
	b.OK("Opened draft PR: %s", prURL)
}

func parseCSV(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
