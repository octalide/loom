package server

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/detect"
	gh "github.com/octalide/loom/internal/github"
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

	deps, verifyErr := s.gh.GetDependencies(ctx, repo, issueNum)
	if verifyErr != nil {
		b.Warn("Could not verify: %v", verifyErr)
	} else {
		hasDeps := len(deps.BlockedBy) > 0 || len(deps.Blocking) > 0 || deps.Parent != nil || len(deps.SubIssues) > 0
		if hasDeps {
			b.Section("Resulting State")
			if len(deps.BlockedBy) > 0 {
				for _, d := range deps.BlockedBy {
					b.Bullet(fmt.Sprintf("blocked by %s", formatDep(d, repo)))
				}
			}
			if len(deps.Blocking) > 0 {
				for _, d := range deps.Blocking {
					b.Bullet(fmt.Sprintf("blocks %s", formatDep(d, repo)))
				}
			}
			if deps.Parent != nil {
				b.Bullet(fmt.Sprintf("child of %s", formatDep(*deps.Parent, repo)))
			}
			if len(deps.SubIssues) > 0 {
				for _, d := range deps.SubIssues {
					b.Bullet(fmt.Sprintf("parent of %s", formatDep(d, repo)))
				}
			}
		}
		for _, w := range deps.Warnings {
			b.Warn("%s", w)
		}
	}

	return builderResult(b), nil, nil
}

type auditInput struct {
	Fix  bool   `json:"fix,omitempty" jsonschema:"Auto-fix safe issues. Default: false"`
	Repo string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd  string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
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
	cfg := dc.Config

	b := newBuilder()
	b.Header(fmt.Sprintf("Audit: %s", repo))

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
		if settings.DefaultBranch != cfg.Branches.Base {
			b.Warn("Repo: default branch is '%s', expected '%s'", settings.DefaultBranch, cfg.Branches.Base)
			if in.Fix {
				if err := s.gh.SetDefaultBranch(ctx, repo, cfg.Branches.Base); err == nil {
					fixed = append(fixed, fmt.Sprintf("Set default branch to '%s'", cfg.Branches.Base))
				}
			}
		} else {
			b.OK("Repo: default branch is '%s'", settings.DefaultBranch)
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
				if err := s.gh.SetBranchProtection(ctx, repo, branch, nil, cfg.IsStrict()); err == nil {
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
					if err := s.gh.SetBranchProtection(ctx, repo, branch, bp.StatusChecks, cfg.IsStrict()); err == nil {
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

	// PR health
	if s.gh != nil {
		openPRs, err := s.gh.ListOpenPRs(ctx, repo)
		if err == nil && len(openPRs) > 0 {
			b.Section(fmt.Sprintf("PR Health (%d open)", len(openPRs)))
			now := time.Now()
			branchPattern := regexp.MustCompile(`^(\w+)/(\d+)`)
			for _, pr := range openPRs {
				var issues []string
				createdAt, _ := time.Parse("2006-01-02T15:04:05Z", pr.CreatedAt)
				updatedAt, _ := time.Parse("2006-01-02T15:04:05Z", pr.UpdatedAt)
				age := now.Sub(createdAt)
				idle := now.Sub(updatedAt)

				if pr.IsDraft && age.Hours() > 7*24 {
					issues = append(issues, fmt.Sprintf("draft for %d days", int(age.Hours()/24)))
				}

				if !pr.IsDraft && idle.Hours() > 7*24 {
					issues = append(issues, fmt.Sprintf("idle for %d days", int(idle.Hours()/24)))
				}

				missingCloseRef := !containsCloseRef(pr.Body, branchPattern, pr.HeadRefName)
				if missingCloseRef {
					issues = append(issues, "missing Closes #N in body")
					if in.Fix {
						if m := branchPattern.FindStringSubmatch(pr.HeadRefName); m != nil {
							issueNum, _ := strconv.Atoi(m[2])
							if issueNum > 0 {
								newBody := pr.Body
								if newBody != "" {
									newBody += "\n\n"
								}
								newBody += fmt.Sprintf("Closes #%d", issueNum)
								if err := s.gh.UpdatePRBody(ctx, repo, pr.Number, newBody); err == nil {
									fixed = append(fixed, fmt.Sprintf("Added 'Closes #%d' to PR #%d body", issueNum, pr.Number))
								}
							}
						}
					}
				}

				readiness, err := s.gh.AssessMergeReadiness(ctx, repo, pr.Number)
				if err == nil {
					if !pr.IsDraft && !readiness.AutoMerge {
						issues = append(issues, "auto-merge not enabled")
						if in.Fix {
							if err := s.gh.EnableAutoMerge(ctx, repo, pr.Number, cfg.MergeMethod); err == nil {
								fixed = append(fixed, fmt.Sprintf("Enabled auto-merge on PR #%d", pr.Number))
							}
						}
					}
					hasFailure := false
					for _, check := range readiness.Checks {
						if check.Conclusion == "failure" || check.Conclusion == "error" {
							hasFailure = true
							break
						}
					}
					if hasFailure {
						issues = append(issues, "failing CI")
					}
					if readiness.MergeState == "dirty" {
						issues = append(issues, "merge conflicts")
					}
				}

				if len(issues) > 0 {
					b.Warn("PR #%d (%s): %s", pr.Number, pr.HeadRefName, strings.Join(issues, ", "))
				} else {
					b.OK("PR #%d (%s)", pr.Number, pr.HeadRefName)
				}
			}
		}

		// Issue health
		openIssues, err := s.gh.ListOpenIssues(ctx, repo)
		if err == nil && len(openIssues) > 0 {
			b.Section(fmt.Sprintf("Issue Health (%d open)", len(openIssues)))
			now := time.Now()
			for _, issue := range openIssues {
				var issues []string
				updatedAt, _ := time.Parse("2006-01-02T15:04:05Z", issue.UpdatedAt)
				idle := now.Sub(updatedAt)

				if len(issue.Labels) == 0 {
					issues = append(issues, "no labels")
				}

				pr, prErr := s.gh.FindPRForIssue(ctx, repo, issue.Number)
				if prErr != nil && idle.Hours() > 14*24 {
					issues = append(issues, fmt.Sprintf("no linked PR, idle for %d days", int(idle.Hours()/24)))
				} else if prErr == nil && pr.State == "closed" {
					issues = append(issues, fmt.Sprintf("has merged PR #%d but wasn't closed", pr.Number))
					if in.Fix {
						if err := s.gh.CloseIssue(ctx, repo, issue.Number); err == nil {
							fixed = append(fixed, fmt.Sprintf("Closed issue #%d (merged PR #%d)", issue.Number, pr.Number))
						}
					}
				}

				if len(issues) > 0 {
					b.Warn("Issue #%d (%s): %s", issue.Number, issue.Title, strings.Join(issues, ", "))
				}
			}
		}

		// Relationship integrity
		if openIssues != nil && len(openIssues) > 0 {
			var relIssues []string
			for _, issue := range openIssues {
				deps, depsErr := s.gh.GetDependencies(ctx, repo, issue.Number)
				if depsErr != nil {
					continue
				}
				for _, w := range deps.Warnings {
					relIssues = append(relIssues, fmt.Sprintf("Issue #%d: %s", issue.Number, w))
				}
				for _, d := range deps.BlockedBy {
					if strings.EqualFold(d.State, "CLOSED") || strings.EqualFold(d.State, "closed") {
						relIssues = append(relIssues, fmt.Sprintf("Issue #%d: blocked by #%d which is closed (stale link)", issue.Number, d.Number))
					}
				}
				for _, d := range deps.Blocking {
					if strings.EqualFold(d.State, "CLOSED") || strings.EqualFold(d.State, "closed") {
						relIssues = append(relIssues, fmt.Sprintf("Issue #%d: blocks #%d which is closed (stale link)", issue.Number, d.Number))
					}
				}
			}
			if len(relIssues) > 0 {
				b.Section(fmt.Sprintf("Relationship Integrity (%d issues)", len(relIssues)))
				for _, ri := range relIssues {
					b.Warn("%s", ri)
				}
			}
		}

		// Workflow integrity — branch naming + orphaned branches
		remoteBranches, err := s.git.ListRemoteBranches(dc.Cwd)
		if err == nil {
			var badBranches []string
			var orphanedBranches []string
			branchTypePattern := regexp.MustCompile(`^(\w+)/(\d+)$`)
			for _, br := range remoteBranches {
				if br == cfg.Branches.Base || br == cfg.Branches.Release || br == "HEAD" {
					continue
				}
				m := branchTypePattern.FindStringSubmatch(br)
				if m == nil {
					badBranches = append(badBranches, br)
					continue
				}
				issueNum, _ := strconv.Atoi(m[2])
				if issueNum > 0 {
					issue, err := s.gh.GetIssue(ctx, repo, issueNum)
					if err == nil && issue.State == "closed" {
						orphanedBranches = append(orphanedBranches, br)
						if in.Fix {
							if err := s.git.DeleteRemoteBranch(dc.Cwd, br); err == nil {
								fixed = append(fixed, fmt.Sprintf("Deleted orphaned remote branch '%s' (issue #%d closed)", br, issueNum))
							}
						}
					}
				}
			}
			hasIntegrityIssues := len(badBranches) > 0 || len(orphanedBranches) > 0
			if hasIntegrityIssues {
				b.Section("Workflow Integrity")
			}
			if len(badBranches) > 0 {
				b.Warn("Branches not following naming convention (%d):", len(badBranches))
				for _, br := range badBranches {
					b.Bullet(fmt.Sprintf("`%s` — expected `{type}/{issue_number}`", br))
				}
			}
			if len(orphanedBranches) > 0 {
				b.Warn("Orphaned remote branches (%d) — issue closed but branch remains:", len(orphanedBranches))
				for _, br := range orphanedBranches {
					b.Bullet(fmt.Sprintf("`%s`", br))
				}
			}
		}
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
	Cleanup bool   `json:"cleanup,omitempty" jsonschema:"Remove GitHub default labels that don't align with conventions. Default: false"`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
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

	// Default branch
	if err := s.gh.SetDefaultBranch(ctx, repo, cfg.Branches.Base); err != nil {
		b.Warn("Default branch failed: %v", err)
	} else {
		b.OK("Repo: default branch set to '%s'", cfg.Branches.Base)
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
		if err := s.gh.SetBranchProtection(ctx, repo, branch, checks, cfg.IsStrict()); err != nil {
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

	// Labels — ensure convention labels, then build inventory
	created, err := s.gh.EnsureLabels(ctx, repo, gh.DefaultLabels)
	if err != nil {
		b.Warn("Labels failed: %v", err)
	} else if len(created) > 0 {
		b.OK("Labels: created %s", strings.Join(created, ", "))
	} else {
		b.OK("Labels: all defaults present")
	}

	allLabels, labelErr := s.gh.ListLabels(ctx, repo)

	conventionNames := make(map[string]bool)
	for _, l := range gh.DefaultLabels {
		conventionNames[l.Name] = true
	}
	githubDefaultNames := make(map[string]bool)
	for _, name := range gh.GitHubDefaultLabels {
		githubDefaultNames[name] = true
	}

	if labelErr == nil {
		var githubDefaults, custom []string
		for _, l := range allLabels {
			if conventionNames[l.Name] {
				continue
			}
			if githubDefaultNames[l.Name] {
				githubDefaults = append(githubDefaults, l.Name)
			} else {
				custom = append(custom, l.Name)
			}
		}

		b.Section("Label Inventory")
		b.Text("**Convention labels** (loom workflow):")
		for _, l := range gh.DefaultLabels {
			b.Bullet(fmt.Sprintf("`%s` — %s", l.Name, l.Description))
		}

		if len(githubDefaults) > 0 {
			if in.Cleanup {
				deleted, delErr := s.gh.DeleteGitHubDefaults(ctx, repo)
				if delErr != nil {
					b.Warn("Failed to remove some GitHub defaults: %v", delErr)
				}
				if len(deleted) > 0 {
					b.OK("Removed GitHub defaults: %s", strings.Join(deleted, ", "))
				}
			} else {
				b.Text("")
				b.Text("**GitHub defaults** (don't align with conventions — recommend removing):")
				for _, name := range githubDefaults {
					b.Bullet(fmt.Sprintf("`%s`", name))
				}
			}
		}

		if len(custom) > 0 {
			b.Text("")
			b.Text("**Custom labels** (already existed):")
			for _, name := range custom {
				b.Bullet(fmt.Sprintf("`%s`", name))
			}
		}
	}

	// loom.yml check
	hasConfig := false
	if dc.Cwd != "" {
		root, _ := s.git.Root(dc.Cwd)
		if root != "" {
			if _, err := readFile(root, ".github", "loom.yml"); err == nil {
				hasConfig = true
			}
		}
	}

	b.Section("Agent Instructions")
	b.Text("Present the label inventory above to the user and walk through these steps:")
	b.Text("")
	step := 1
	if !in.Cleanup {
		b.Text(fmt.Sprintf("%d. **Remove GitHub defaults**: The labels like `bug`, `enhancement`, `documentation` overlap with convention labels (`fix`, `feat`, `doc`). Ask the user if they want them removed. Use `labels(action: \"delete_defaults\")` to remove all at once, or `labels(action: \"delete\", name: \"...\")` individually.", step))
		step++
	}
	b.Text(fmt.Sprintf("%d. **Project-specific labels**: Ask the user if they need labels for their project. Suggest common categories:", step))
	step++
	b.Bullet("**Area labels** — e.g. `frontend`, `backend`, `api`, `infra`, `database`")
	b.Bullet("**Priority labels** — e.g. `p0`, `p1`, `p2` or `critical`, `high`, `low`")
	b.Bullet("**Qualifier labels** — e.g. `breaking-change`, `security`, `blocked`, `needs-design`")
	b.Text(fmt.Sprintf("%d. Use `labels(action: \"create\", name: \"...\", description: \"...\", color: \"...\")` to create labels the user wants.", step))
	step++
	if !hasConfig {
		b.Text(fmt.Sprintf("%d. **Create `.github/loom.yml`**: This repo has no loom config. Offer to create one with the detected settings (base branch: %s, release branch: %s). Include any CI checks if workflows were found.", step, cfg.Branches.Base, cfg.Branches.Release))
	}

	return builderResult(b), nil, nil
}

type labelsInput struct {
	Action      string `json:"action" jsonschema:"Action: list create or delete"`
	Name        string `json:"name,omitempty" jsonschema:"Label name (required for create/delete)"`
	Description string `json:"description,omitempty" jsonschema:"Label description (for create)"`
	Color       string `json:"color,omitempty" jsonschema:"Hex color without # (for create). Default: 6e7781"`
	Repo        string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd         string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleLabels(ctx context.Context, req *mcp.CallToolRequest, in labelsInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	b := newBuilder()

	switch in.Action {
	case "list":
		labels, err := s.gh.ListLabels(ctx, repo)
		if err != nil {
			return errorResult("failed to list labels: %v", err), nil, nil
		}
		b.Header(fmt.Sprintf("Labels: %s (%d)", repo, len(labels)))
		for _, l := range labels {
			desc := ""
			if l.Description != "" {
				desc = fmt.Sprintf(" — %s", l.Description)
			}
			b.Bullet(fmt.Sprintf("`%s`%s (#%s)", l.Name, desc, l.Color))
		}
		if len(labels) == 0 {
			b.Text("No labels")
		}

	case "create":
		if in.Name == "" {
			return errorResult("name is required for create"), nil, nil
		}
		color := in.Color
		if color == "" {
			color = "6e7781"
		}
		label := gh.Label{Name: in.Name, Description: in.Description, Color: color}
		if err := s.gh.CreateLabel(ctx, repo, label); err != nil {
			return errorResult("failed to create label: %v", err), nil, nil
		}
		b.OK("Created label `%s` (#%s)", in.Name, color)

	case "delete":
		if in.Name == "" {
			return errorResult("name is required for delete"), nil, nil
		}
		if err := s.gh.DeleteLabel(ctx, repo, in.Name); err != nil {
			return errorResult("failed to delete label: %v", err), nil, nil
		}
		b.OK("Deleted label `%s`", in.Name)

	case "delete_defaults":
		deleted, err := s.gh.DeleteGitHubDefaults(ctx, repo)
		if err != nil {
			return errorResult("failed to delete defaults: %v", err), nil, nil
		}
		if len(deleted) > 0 {
			b.OK("Removed GitHub default labels: %s", strings.Join(deleted, ", "))
		} else {
			b.OK("No GitHub default labels found to remove")
		}

	default:
		return errorResult("action must be list, create, delete, or delete_defaults; got %q", in.Action), nil, nil
	}

	return builderResult(b), nil, nil
}

type releaseInput struct {
	Action  string `json:"action" jsonschema:"Action: prepare or execute"`
	Version string `json:"version,omitempty" jsonschema:"Version tag (required for execute, e.g. v1.2.0)"`
	Notes   string `json:"notes,omitempty" jsonschema:"Release notes body (for execute). If omitted, uses the commit changelog from prepare."`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleRelease(ctx context.Context, req *mcp.CallToolRequest, in releaseInput) (*mcp.CallToolResult, any, error) {
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

	switch in.Action {
	case "prepare":
		b.Header(fmt.Sprintf("Release Preparation: %s", repo))

		latestTag, err := s.git.LatestTag(dc.Cwd)
		if err != nil {
			latestTag = "(none)"
		}
		b.KV("Current version", latestTag)
		b.KV("Base branch", cfg.Branches.Base)
		b.KV("Release branch", cfg.Branches.Release)

		_ = s.git.Fetch(dc.Cwd, "origin", cfg.Branches.Base)
		_ = s.git.Fetch(dc.Cwd, "origin", cfg.Branches.Release)

		ahead, _ := s.git.CommitCountBetween(dc.Cwd, "origin/"+cfg.Branches.Release, "origin/"+cfg.Branches.Base)
		if ahead == 0 {
			b.Info("%s is up to date with %s — nothing to release", cfg.Branches.Base, cfg.Branches.Release)
			return builderResult(b), nil, nil
		}
		b.OK("%s is %d commit(s) ahead of %s", cfg.Branches.Base, ahead, cfg.Branches.Release)

		tagRef := latestTag
		if latestTag == "(none)" {
			tagRef = ""
		}
		commits, _ := s.git.CommitsSinceTag(dc.Cwd, tagRef)

		groups := map[string][]string{}
		for _, c := range commits {
			parts := strings.SplitN(c, " ", 2)
			if len(parts) < 2 {
				continue
			}
			msg := parts[1]
			ctype := "other"
			if idx := strings.Index(msg, ":"); idx > 0 {
				ctype = msg[:idx]
			}
			groups[ctype] = append(groups[ctype], msg)
		}

		if len(commits) > 0 {
			b.Section(fmt.Sprintf("Changes since %s (%d commits)", latestTag, len(commits)))
			order := []string{"feat", "fix", "refactor", "doc", "test", "chore", "build", "ci", "perf"}
			shown := map[string]bool{}
			for _, t := range order {
				if msgs, ok := groups[t]; ok {
					b.Text(fmt.Sprintf("**%s**:", t))
					for _, m := range msgs {
						b.Bullet(m)
					}
					shown[t] = true
				}
			}
			for t, msgs := range groups {
				if shown[t] {
					continue
				}
				b.Text(fmt.Sprintf("**%s**:", t))
				for _, m := range msgs {
					b.Bullet(m)
				}
			}
		}

		ciState, checks, _ := s.gh.GetBranchCIStatus(ctx, repo, cfg.Branches.Base)
		b.Section("CI Status")
		if ciState == "" && len(checks) == 0 {
			b.Info("No CI checks found on %s", cfg.Branches.Base)
		} else {
			b.KV("Overall", ciState)
			for _, c := range checks {
				status := c.Conclusion
				if status == "" {
					status = c.Status
				}
				if status == "success" {
					b.OK("%s", c.Name)
				} else if status == "failure" || status == "error" {
					b.Warn("%s: %s", c.Name, status)
				} else {
					b.Info("%s: %s", c.Name, status)
				}
			}
		}

		b.Section("Agent Instructions")
		b.Text("Review the changes and CI status above with the user, then:")
		b.Text("1. Decide on a version number (semver). Consider: feat commits → minor bump, fix-only → patch bump, breaking changes → major bump.")
		b.Text("2. If CI is not passing or not present, confirm with the user whether to proceed.")
		b.Text("3. Optionally draft release notes — the commit list above can serve as a starting point.")
		b.Text(fmt.Sprintf("4. Call `release(action: \"execute\", version: \"vX.Y.Z\")` to merge %s → %s, tag, and create a GitHub release.", cfg.Branches.Base, cfg.Branches.Release))

	case "execute":
		if in.Version == "" {
			return errorResult("version is required for execute (e.g. v1.2.0)"), nil, nil
		}
		if !strings.HasPrefix(in.Version, "v") {
			return errorResult("version must start with 'v' (e.g. v1.2.0)"), nil, nil
		}

		b.Header(fmt.Sprintf("Release: %s %s", repo, in.Version))

		if err := s.git.CheckoutAndPull(dc.Cwd, cfg.Branches.Release); err != nil {
			return errorResult("failed to checkout %s: %v", cfg.Branches.Release, err), nil, nil
		}
		b.OK("Checked out %s", cfg.Branches.Release)

		if err := s.git.MergeBranch(dc.Cwd, "origin/"+cfg.Branches.Base); err != nil {
			return errorResult("failed to merge %s → %s: %v", cfg.Branches.Base, cfg.Branches.Release, err), nil, nil
		}
		b.OK("Merged %s → %s", cfg.Branches.Base, cfg.Branches.Release)

		if err := s.git.Push(dc.Cwd, cfg.Branches.Release); err != nil {
			return errorResult("failed to push %s: %v", cfg.Branches.Release, err), nil, nil
		}
		b.OK("Pushed %s", cfg.Branches.Release)

		if err := s.git.Tag(dc.Cwd, in.Version); err != nil {
			return errorResult("failed to create tag %s: %v", in.Version, err), nil, nil
		}
		if err := s.git.PushTag(dc.Cwd, in.Version); err != nil {
			return errorResult("failed to push tag %s: %v", in.Version, err), nil, nil
		}
		b.OK("Tagged %s", in.Version)

		notes := in.Notes
		if notes == "" {
			latestTag, _ := s.git.LatestTag(dc.Cwd)
			tagRef := ""
			if latestTag != in.Version {
				tagRef = latestTag
			}
			commits, _ := s.git.CommitsSinceTag(dc.Cwd, tagRef)
			if len(commits) > 0 {
				var lines []string
				for _, c := range commits {
					parts := strings.SplitN(c, " ", 2)
					if len(parts) >= 2 {
						lines = append(lines, "- "+parts[1])
					}
				}
				notes = strings.Join(lines, "\n")
			}
		}

		url, err := s.gh.CreateRelease(ctx, repo, in.Version, in.Version, notes)
		if err != nil {
			b.Warn("GitHub release failed: %v", err)
			b.Text("Tag was pushed successfully. Create the release manually if needed.")
		} else {
			b.OK("Created release: %s", url)
		}

		_ = s.git.CheckoutAndPull(dc.Cwd, cfg.Branches.Base)
		b.OK("Checked out %s", cfg.Branches.Base)

	default:
		return errorResult("action must be prepare or execute; got %q", in.Action), nil, nil
	}

	return builderResult(b), nil, nil
}

func containsCloseRef(body string, branchPattern *regexp.Regexp, branchName string) bool {
	issueNum := 0
	if m := branchPattern.FindStringSubmatch(branchName); m != nil {
		issueNum, _ = strconv.Atoi(m[2])
	}
	if issueNum == 0 {
		return true // can't determine expected issue, don't flag
	}
	ref := fmt.Sprintf("#%d", issueNum)
	lower := strings.ToLower(body)
	return strings.Contains(lower, "closes "+ref) ||
		strings.Contains(lower, "fixes "+ref) ||
		strings.Contains(lower, "resolves "+ref)
}

