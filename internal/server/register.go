package server

import "github.com/modelcontextprotocol/go-sdk/mcp"

func (s *Server) registerTools() {
	// Workflow lifecycle
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a new GitHub Project (V2). Returns project number and URL.",
	}, s.handleCreateProject)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a GitHub issue, add it to a project board, and set status to Todo. Auto-detects repo and project from .github/loom.yml.",
	}, s.handleCreateIssue)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "start",
		Description: "Start working on an issue: create branch from base, push, set project status to In Progress. Auto-detects repo and project.",
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
		Description: "Get current workflow status: branch, uncommitted changes, open PRs, project board. Auto-detects everything.",
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
		Name:        "board_status",
		Description: `Manually update project board status for an issue. Status must be "Todo", "In Progress", or "Done" (or as configured in .github/loom.yml).`,
	}, s.handleBoardStatus)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "audit",
		Description: "Audit a repo for workflow compliance: auth, repo settings, branch protection, stale branches, worktrees. fix=true auto-fixes safe issues.",
	}, s.handleAudit)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "setup",
		Description: "Configure a repo with the standard branch workflow: base + release branches, branch protection, auto-delete, auto-merge.",
	}, s.handleSetup)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "worktrees",
		Description: "List all git worktrees with path, branch, and issue number.",
	}, s.handleWorktrees)
}
