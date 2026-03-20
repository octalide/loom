# Loom — GitHub Workflow MCP Server

Loom enforces a standard GitHub workflow: every code change starts with an issue, every issue gets a branch, every branch gets a PR, every PR merges via auto-merge. Project boards track status automatically.

## Configuration

Place `.github/loom.yml` in the repo root to configure defaults. All fields are optional:

```yaml
project: 5              # GitHub Projects V2 number
branches:
  base: "dev"           # base branch for feature branches
  release: "main"       # release branch
statuses:
  todo: "Todo"
  in_progress: "In Progress"
  done: "Done"
```

Without this file, repo and branch names are auto-detected from git state. Project number must be passed explicitly if not configured.

## Tools

### Core Workflow

- **`create_project(title, owner?)`** — Create a new GitHub Project V2
- **`create_issue(title, body, repo?, project?, labels?)`** — Create issue → add to board → set Todo
- **`start(issue, branch_type?, worktree?, cwd?)`** — Checkout base → create branch → push → set In Progress
- **`commit(message, files?, push?, cwd?)`** — Stage → commit → push. Auto-creates draft PR on first push. Refuses base/release branches.
- **`finish(issue?, cwd?)`** — Push → ready PR → enable auto-merge (squash) → cleanup branch/worktree

### Observability

- **`status(repo?, project?)`** — Current branch, uncommitted changes, open PRs, board status, attention-needed alerts (failing CI, stale PRs, missing close refs). Zero required params.
- **`context(issue, repo?)`** — Full issue details: description, comments, labels, linked PR, dependencies, merge readiness
- **`activity(issue, since?, repo?)`** — Recent activity on issue + linked PR. Filter with ISO 8601 timestamp.
- **`pr_feedback(pr, repo?)`** — Reviews, inline comments grouped by file, CI status, merge readiness
- **`dependencies(issue, repo?)`** — All relationships: blocked-by, blocking, parent, sub-issues

### Relationships

- **`link(issue, target, relationship, target_repo?, repo?)`** — Create or remove issue relationships
  - Relationships: `blocked_by`, `blocks`, `parent_of`, `child_of`
  - Prefix with `-` to remove (e.g. `-blocked_by`)

### Admin

- **`board_status(issue, status, repo?, project?)`** — Manual board status override
- **`audit(fix?, repo?, project?)`** — Repo health: infrastructure compliance, PR health (failing CI, stale drafts, idle PRs, missing close refs, auto-merge), issue health (unlabeled, idle, no linked PR), workflow integrity (branch naming). `fix=true` auto-fixes safe issues including adding `Closes #N` to PR bodies and enabling auto-merge.
- **`setup(cleanup?, repo?)`** — Configure new repo: branches, protection, auto-delete, auto-merge, convention labels. Returns label inventory and agent instructions to interactively guide the user through removing GitHub defaults, adding project-specific labels, and creating loom.yml. `cleanup=true` auto-removes GitHub default labels.
- **`labels(action, name?, description?, color?, repo?)`** — Manage repo labels. Actions: `list`, `create`, `delete`, `delete_defaults` (batch remove GitHub defaults).
- **`worktrees(cwd?)`** — List all worktrees with branch and issue number

## Auto-Detection

Most parameters are optional. Loom detects:
- **repo** from `git remote get-url origin`
- **issue number** from branch name (`feat/42` → 42)
- **project number** from `.github/loom.yml`
- **branch type** from branch name (`feat/42` → feat)

Explicit parameters always override detected values.

## Commit Convention

Commit messages follow: `<type>: <short description>`

Types: `feat`, `fix`, `refactor`, `doc`, `test`, `chore`, `build`, `ci`, `perf`

## Typical Flow

1. `create_issue(title, body)` → creates issue, adds to board, sets Todo
2. `start(issue)` → creates branch, sets In Progress
3. `commit(message)` → stages, commits, pushes, auto-creates draft PR
4. _(repeat commits as needed)_
5. `finish()` → readies PR, enables auto-merge, cleans up locally
