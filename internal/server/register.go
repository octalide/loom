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
		Description: "Create a GitHub issue with optional labels. Auto-detects repo.",
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
		Description: "Get current workflow status: branch, uncommitted changes, open PRs, attention-needed alerts (failing CI, stale PRs, missing close refs, merge conflicts). Auto-detects everything.",
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
		Description: "Audit repo health: infrastructure compliance (auth, settings, protection, default branch), PR health (failing CI, stale drafts, missing auto-merge), issue health (unlabeled, idle, no linked PR), workflow integrity (branch naming, missing Closes #N). fix=true auto-fixes safe issues.",
	}, s.handleAudit)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "setup",
		Description: "Configure a new repo: branch workflow, protection rules, auto-delete, auto-merge, default branch, convention labels. Returns a label inventory and agent instructions — walk the user through removing GitHub defaults, adding project-specific labels, and creating loom.yml. Pass cleanup=true to auto-remove GitHub default labels.",
	}, s.handleSetup)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "labels",
		Description: "Manage repo labels. Actions: list, create, delete, delete_defaults (removes all GitHub default labels). Use after setup to customize labels for the project.",
	}, s.handleLabels)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "worktrees",
		Description: "List all git worktrees with path, branch, and issue number.",
	}, s.handleWorktrees)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "release",
		Description: "Prepare or execute a release. prepare: gathers changes since last tag, CI status, and suggests next steps. execute: merges base → release, tags, creates GitHub release. Version decisions are left to the agent and user.",
	}, s.handleRelease)
}
