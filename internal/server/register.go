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
		Description: "Get current workflow status: branch, uncommitted changes, open PRs, project board, attention-needed alerts (failing CI, stale PRs, missing close refs). Auto-detects everything.",
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
		Description: "Audit repo health: infrastructure compliance (auth, settings, protection), PR health (failing CI, stale drafts, missing auto-merge), issue health (unlabeled, idle, no linked PR), workflow integrity (branch naming, missing Closes #N). fix=true auto-fixes safe issues.",
	}, s.handleAudit)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "setup",
		Description: "Configure a new repo: branch workflow, protection rules, auto-delete, auto-merge, convention labels. Returns a label inventory and agent instructions — walk the user through removing GitHub defaults, adding project-specific labels, and creating loom.yml. Pass cleanup=true to auto-remove GitHub default labels.",
	}, s.handleSetup)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "labels",
		Description: "Manage repo labels. Actions: list, create, delete, delete_defaults (removes all GitHub default labels). Use after setup to customize labels for the project.",
	}, s.handleLabels)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "worktrees",
		Description: "List all git worktrees with path, branch, and issue number.",
	}, s.handleWorktrees)
}
