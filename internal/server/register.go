package server

import "github.com/modelcontextprotocol/go-sdk/mcp"

func (s *Server) registerTools() {
	// Meta
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "usage",
		Description: "Get comprehensive documentation: workflow lifecycle, config reference, tool descriptions, conventions. Call this when unsure how to use loom.",
	}, s.handleUsage)

	// Workflow lifecycle
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a GitHub issue with optional labels. Auto-detects repo from git remote.",
	}, s.handleCreateIssue)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "start",
		Description: "Start working on an issue: create branch from base, push. Auto-detects repo.",
	}, s.handleStart)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "commit",
		Description: "Commit changes on the current feature branch and push. Auto-creates draft PR on first push. Refuses to commit on base or release branches.",
	}, s.handleCommit)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "finish",
		Description: "Finish an issue: push, ready PR, enable auto-merge, clean up branch/worktree. Auto-detects issue from branch name.",
	}, s.handleFinish)

	// Observability
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "status",
		Description: "Get current workflow status: branch, uncommitted changes, open PRs. Auto-detects everything.",
	}, s.handleStatus)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "context",
		Description: "Get full context for an issue: description, comments, labels, linked PR, dependencies, merge readiness.",
	}, s.handleContext)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "activity",
		Description: "Check for recent activity on an issue and its linked PR. Optionally filter by timestamp.",
	}, s.handleActivity)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "pr_feedback",
		Description: "Get all feedback on a PR: reviews, inline comments grouped by file, CI status, merge readiness.",
	}, s.handlePRFeedback)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "dependencies",
		Description: "Show all dependency and hierarchy relationships for an issue: blocked-by, blocking, parent, sub-issues.",
	}, s.handleDependencies)

	// Relationships
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "link",
		Description: `Create or remove a relationship between issues. Relationships: "blocked_by", "blocks", "parent_of", "child_of". Prefix with "-" to remove.`,
	}, s.handleLink)

	// Admin
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "audit",
		Description: "Audit a repo for workflow compliance: auth, repo settings, branch protection, stale branches, worktrees. fix=true auto-fixes safe issues.",
	}, s.handleAudit)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "setup",
		Description: "Configure a repo with the standard branch workflow: base + release branches, branch protection, auto-delete, auto-merge. CI checks are read from .github/loom.yml.",
	}, s.handleSetup)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "worktrees",
		Description: "List all git worktrees with path, branch, and issue number.",
	}, s.handleWorktrees)
}
