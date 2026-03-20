package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type usageInput struct{}

func (s *Server) handleUsage(ctx context.Context, req *mcp.CallToolRequest, in usageInput) (*mcp.CallToolResult, any, error) {
	b := newBuilder()

	b.Header("Loom — GitHub Workflow MCP Server")
	b.Text("Loom enforces a standard GitHub workflow: every code change starts with an issue, every issue gets a branch, every branch gets a PR, every PR merges via auto-merge.")

	b.Section("Workflow")
	b.Text("1. `create_issue` — create issue, add to project board, set Todo")
	b.Text("2. `start` — create branch from base, push, set In Progress")
	b.Text("3. `commit` — stage, commit, push (auto-creates draft PR on first push)")
	b.Text("4. Repeat `commit` as needed")
	b.Text("5. `finish` — ready PR, enable auto-merge (squash), clean up locally")
	b.Text("")
	b.Text("After `finish`, GitHub handles the rest: CI runs, PR merges when checks pass, issue closes automatically (PR body contains `Closes #N`), project board updates via GitHub Projects automation.")

	b.Section("Auto-Detection")
	b.Text("Most parameters are optional. Loom detects:")
	b.Bullet("`repo` — from git remote URL")
	b.Bullet("`issue` — from branch name (e.g. `feat/42` → 42)")
	b.Bullet("`project` — from `.github/loom.yml`")
	b.Bullet("`owner` — from repo string")
	b.Bullet("`branch_type` — defaults to `feat`")
	b.Text("")
	b.Text("Pass `cwd` to any tool when operating on a repo outside the current working directory.")
	b.Text("Explicit parameters always override detected values.")

	b.Section("Config (.github/loom.yml)")
	b.Text("Optional. Place in the repo's `.github/` directory. All fields have defaults.")
	b.Text("```yaml")
	b.Text("project: 5              # GitHub Projects V2 number (required for board ops)")
	b.Text("")
	b.Text("branches:")
	b.Text("  base: \"dev\"           # base branch for feature branches (default: dev)")
	b.Text("  release: \"main\"       # release branch (default: main)")
	b.Text("  types:                # allowed branch prefixes")
	b.Text("    - feat")
	b.Text("    - fix")
	b.Text("    - doc")
	b.Text("    - refactor")
	b.Text("    - issue")
	b.Text("")
	b.Text("statuses:               # project board column names")
	b.Text("  todo: \"Todo\"")
	b.Text("  in_progress: \"In Progress\"")
	b.Text("  done: \"Done\"")
	b.Text("")
	b.Text("checks:                 # CI status checks required for branch protection")
	b.Text("  - \"build (linux, amd64)\"")
	b.Text("  - vet")
	b.Text("  - test")
	b.Text("```")

	b.Section("Tools")
	b.Text("**Workflow lifecycle:**")
	b.Bullet("`create_project(title)` — new GitHub Project V2")
	b.Bullet("`create_issue(title, body)` — create + board + Todo")
	b.Bullet("`start(issue)` — branch + push + In Progress")
	b.Bullet("`commit(message)` — stage + commit + push + auto-draft-PR")
	b.Bullet("`finish()` — ready PR + auto-merge + cleanup")
	b.Text("")
	b.Text("**Observability (read-only):**")
	b.Bullet("`status()` — branch, changes, open PRs, board")
	b.Bullet("`context(issue)` — full issue details + linked PR + deps")
	b.Bullet("`activity(issue)` — recent comments, reviews, CI status")
	b.Bullet("`pr_feedback(pr)` — reviews, inline comments, merge readiness")
	b.Bullet("`dependencies(issue)` — blocked-by, blocking, parent, sub-issues")
	b.Text("")
	b.Text("**Relationships:**")
	b.Bullet("`link(issue, target, relationship)` — blocked_by, blocks, parent_of, child_of (prefix with `-` to remove)")
	b.Text("")
	b.Text("**Admin:**")
	b.Bullet("`board_status(issue, status)` — manual board status override")
	b.Bullet("`audit()` — check workflow compliance, fix=true auto-fixes")
	b.Bullet("`setup()` — configure new repo: branches, protection, labels, then guides you through project-specific customization")
	b.Bullet("`labels(action, name?, description?, color?)` — list, create, or delete repo labels")
	b.Bullet("`worktrees()` — list worktrees with issue numbers")

	b.Section("Conventions")
	b.Bullet("Branch naming: `{type}/{issue_number}` (e.g. `feat/42`, `fix/17`)")
	b.Bullet("Commit messages: `<type>: <short description>` (feat, fix, refactor, doc, test, chore, build, ci, perf)")
	b.Bullet("PR body: `Closes #N` (auto-closes the issue on merge)")
	b.Bullet("Merge strategy: squash")
	b.Bullet("One issue = one branch = one PR")

	return builderResult(b), nil, nil
}
