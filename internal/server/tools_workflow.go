package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/detect"
	"github.com/octalide/loom/internal/output"
)

type createIssueInput struct {
	Title  string `json:"title" jsonschema:"Issue title"`
	Body   string `json:"body" jsonschema:"Issue body (markdown)"`
	Repo   string `json:"repo,omitempty" jsonschema:"Repository in owner/repo format. Auto-detected if omitted."`
	Labels string `json:"labels,omitempty" jsonschema:"Comma-separated label names to apply"`
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

	return builderResult(b), nil, nil
}

type commentInput struct {
	Issue string `json:"issue,omitempty" jsonschema:"Issue number. Auto-detected from branch name if omitted."`
	Body  string `json:"body" jsonschema:"Comment body (markdown)"`
	Repo  string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd   string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleComment(ctx context.Context, req *mcp.CallToolRequest, in commentInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	issueNum := detect.FirstNonZero(parseInt(in.Issue), dc.IssueNumber)
	if issueNum == 0 {
		return errorResult("could not detect issue number; pass explicitly"), nil, nil
	}

	taggedBody := in.Body + "\n\n<!-- loom:comment -->"
	url, err := s.gh.CreateIssueComment(ctx, repo, issueNum, taggedBody)
	if err != nil {
		return errorResult("failed to post comment: %v", err), nil, nil
	}

	b := newBuilder()
	b.OK("Comment posted on #%d", issueNum)
	b.KV("URL", url)
	return builderResult(b), nil, nil
}

type startInput struct {
	Issue      string `json:"issue" jsonschema:"Issue number to start working on"`
	Repo       string `json:"repo,omitempty" jsonschema:"Repository in owner/repo format. Auto-detected if omitted."`
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
	issue := parseInt(in.Issue)
	cfg := dc.Config

	branchType := in.BranchType
	if branchType == "" {
		branchType = "feat"
	}
	if !cfg.ValidBranchType(branchType) {
		return errorResult("invalid branch_type %q; allowed: %s. Call `usage` for conventions", branchType, strings.Join(cfg.Branches.Types, ", ")), nil, nil
	}

	if !in.Worktree && s.git.HasUncommittedChanges(dc.Cwd) {
		return errorResult("uncommitted changes detected; commit or stash before starting a new issue"), nil, nil
	}

	// Fetch issue details
	ghIssue, err := s.gh.GetIssue(ctx, repo, issue)
	if err != nil {
		return errorResult("could not fetch issue #%d: %v", issue, err), nil, nil
	}

	branchName := fmt.Sprintf("%s/%d", branchType, issue)
	b := newBuilder()
	b.Header(fmt.Sprintf("Issue #%d: %s", issue, ghIssue.Title))

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
		return errorResult("refusing to commit directly on '%s' — use `start(issue)` to create a feature branch. Call `usage` for workflow details", branch), nil, nil
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
	Issue string `json:"issue,omitempty" jsonschema:"Issue number. Auto-detected from branch name if omitted."`
	Repo  string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd   string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
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

	issueNumber := detect.FirstNonZero(parseInt(in.Issue), dc.IssueNumber)
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

	cfg := dc.Config

	// Ready PR if draft
	if pr.IsDraft {
		if err := s.gh.ReadyPR(ctx, repo, pr.Number); err != nil {
			b.Warn("failed to ready PR: %v", err)
		} else {
			b.OK("Marked PR as ready")
		}
	}

	// Assess merge readiness to decide strategy
	readiness, err := s.gh.AssessMergeReadiness(ctx, repo, pr.Number)
	merged := false
	if err != nil {
		b.Warn("could not assess merge readiness: %v", err)
	} else {
		hasPending := false
		hasFailing := false
		for _, check := range readiness.Checks {
			if check.Status != "completed" && check.Conclusion == "" {
				hasPending = true
			}
			if check.Conclusion == "failure" || check.Conclusion == "error" {
				hasFailing = true
			}
		}

		switch {
		case hasFailing:
			b.Warn("CI checks are failing — PR will not be merged")
			b.Section("Merge Readiness")
			for _, line := range readiness.Summary {
				b.Bullet(line)
			}

		case hasPending:
			_, err := s.gh.ReadyAndAutoMerge(ctx, repo, pr.Number, cfg.MergeMethod)
			if err != nil {
				b.Warn("failed to enable auto-merge: %v", err)
			} else {
				b.OK("Auto-merge enabled — PR will merge when checks pass")
			}

		default:
			switch readiness.MergeState {
			case "clean", "unstable", "has_hooks", "":
				if err := s.gh.MergePR(ctx, repo, pr.Number, cfg.MergeMethod); err != nil {
					b.Warn("direct merge failed: %v", err)
					_, autoErr := s.gh.ReadyAndAutoMerge(ctx, repo, pr.Number, cfg.MergeMethod)
					if autoErr != nil {
						b.Warn("auto-merge fallback also failed: %v", autoErr)
					} else {
						b.OK("Auto-merge enabled as fallback")
					}
				} else {
					b.OK("Merged PR #%d (%s)", pr.Number, strings.ToLower(cfg.MergeMethod))
					merged = true
				}

			case "behind":
				b.Info("PR branch is behind %s — updating", cfg.Branches.Base)
				if err := s.gh.UpdatePRBranch(ctx, repo, pr.Number); err != nil {
					b.Warn("failed to update PR branch: %v", err)
				} else {
					b.OK("Updated PR branch with latest %s", cfg.Branches.Base)
					_, autoErr := s.gh.ReadyAndAutoMerge(ctx, repo, pr.Number, cfg.MergeMethod)
					if autoErr != nil {
						b.Warn("failed to enable auto-merge after update: %v", autoErr)
					} else {
						b.OK("Auto-merge enabled — PR will merge when CI passes on updated branch")
					}
				}

			case "blocked":
				_, autoErr := s.gh.ReadyAndAutoMerge(ctx, repo, pr.Number, cfg.MergeMethod)
				if autoErr != nil {
					b.Warn("PR is blocked and auto-merge could not be enabled: %v", autoErr)
				} else {
					b.OK("Auto-merge enabled — PR will merge when requirements are met")
				}

			default:
				b.Warn("PR is not in a mergeable state: %s", readiness.MergeState)
				b.Section("Merge Readiness")
				for _, line := range readiness.Summary {
					b.Bullet(line)
				}
			}
		}
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

	if merged {
		b.Text("")
		b.Text("PR merged. GitHub will automatically close the issue.")
	}

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

type updateIssueInput struct {
	Issue        string `json:"issue,omitempty" jsonschema:"Issue number. Auto-detected from branch name if omitted."`
	Title        string `json:"title,omitempty" jsonschema:"New issue title"`
	Body         string `json:"body,omitempty" jsonschema:"New issue body (replaces entirely)"`
	AddLabels    string `json:"add_labels,omitempty" jsonschema:"Comma-separated labels to add"`
	RemoveLabels string `json:"remove_labels,omitempty" jsonschema:"Comma-separated labels to remove"`
	Repo         string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd          string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleUpdateIssue(ctx context.Context, req *mcp.CallToolRequest, in updateIssueInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	issueNum := detect.FirstNonZero(parseInt(in.Issue), dc.IssueNumber)
	if issueNum == 0 {
		return errorResult("could not detect issue number; pass explicitly"), nil, nil
	}

	if in.Title == "" && in.Body == "" && in.AddLabels == "" && in.RemoveLabels == "" {
		return errorResult("at least one of title, body, add_labels, or remove_labels is required"), nil, nil
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("Update #%d", issueNum))

	if in.Title != "" || in.Body != "" {
		var titlePtr, bodyPtr *string
		if in.Title != "" {
			titlePtr = &in.Title
		}
		if in.Body != "" {
			bodyPtr = &in.Body
		}
		if err := s.gh.UpdateIssue(ctx, repo, issueNum, titlePtr, bodyPtr); err != nil {
			b.Warn("Failed to update issue: %v", err)
		} else {
			if in.Title != "" {
				b.OK("Title updated")
			}
			if in.Body != "" {
				b.OK("Body updated")
			}
		}
	}

	if in.AddLabels != "" {
		labels := parseCSV(in.AddLabels)
		if err := s.gh.AddLabels(ctx, repo, issueNum, labels); err != nil {
			b.Warn("Failed to add labels: %v", err)
		} else {
			b.OK("Added labels: %s", strings.Join(labels, ", "))
		}
	}

	if in.RemoveLabels != "" {
		for _, label := range parseCSV(in.RemoveLabels) {
			if err := s.gh.RemoveLabel(ctx, repo, issueNum, label); err != nil {
				b.Warn("Failed to remove label %q: %v", label, err)
			} else {
				b.OK("Removed label: %s", label)
			}
		}
	}

	return builderResult(b), nil, nil
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

func (s *Server) waitForPRMerge(ctx context.Context, repo string, prNumber int, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pr, err := s.gh.GetPR(ctx, repo, prNumber)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		if pr.Merged {
			return "merged"
		}
		if pr.State == "closed" {
			return "closed"
		}
		time.Sleep(3 * time.Second)
	}
	return "timeout"
}

type waitInput struct {
	PR      string `json:"pr" jsonschema:"PR number to wait for"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"Timeout in seconds. Default: 90"`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleWait(ctx context.Context, req *mcp.CallToolRequest, in waitInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	prNum := parseInt(in.PR)
	if prNum == 0 {
		return errorResult("pr is required"), nil, nil
	}

	timeout := 90
	if in.Timeout > 0 {
		timeout = in.Timeout
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("Waiting for PR #%d", prNum))

	pr, err := s.gh.GetPR(ctx, repo, prNum)
	if err != nil {
		return errorResult("could not fetch PR #%d: %v", prNum, err), nil, nil
	}

	if pr.State == "closed" {
		b.OK("PR #%d is already merged", prNum)
		return builderResult(b), nil, nil
	}

	result := s.waitForPRMerge(ctx, repo, prNum, time.Duration(timeout)*time.Second)
	switch result {
	case "merged":
		b.OK("PR #%d merged", prNum)
	case "closed":
		b.Warn("PR #%d was closed without merging", prNum)
	case "timeout":
		b.Warn("Timed out after %ds — PR #%d is still open", timeout, prNum)
	default:
		b.Warn("PR #%d ended in unexpected state: %s", prNum, result)
	}

	return builderResult(b), nil, nil
}
