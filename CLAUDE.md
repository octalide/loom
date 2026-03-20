# Loom — GitHub Workflow MCP Server

Loom enforces a standard GitHub workflow: every code change starts with an issue, every issue gets a branch, every branch gets a PR, every PR merges via auto-merge.

## Configuration

Place `.github/loom.yml` in the repo root to configure defaults. All fields are optional:

```yaml
branches:
  base: "dev"           # base branch for feature branches
  release: "main"       # release branch
merge_method: "squash"  # merge strategy: merge, squash, rebase (default: merge)
checks:                 # CI status checks required for branch protection
  - build
  - test
```

Without this file, repo and branch names are auto-detected from git state.

## Tools

### Core Workflow

- **`create_issue(title, body, repo?, labels?)`** — Create issue with optional labels
- **`start(issue, branch_type?, worktree?, cwd?)`** — Checkout base → create branch → push
- **`commit(message, files?, push?, cwd?)`** — Stage → commit → push. Auto-creates draft PR on first push. Refuses base/release branches.
- **`finish(issue?, cwd?)`** — Push → ready PR → enable auto-merge → cleanup branch/worktree

### Observability

- **`status(repo?)`** — Current branch, uncommitted changes, open PRs. Zero required params.
- **`context(issue, repo?)`** — Full issue details: description, comments, labels, linked PR, dependencies, merge readiness
- **`activity(issue, since?, repo?)`** — Recent activity on issue + linked PR. Filter with ISO 8601 timestamp.
- **`pr_feedback(pr, repo?)`** — Reviews, inline comments grouped by file, CI status, merge readiness
- **`dependencies(issue, repo?)`** — All relationships: blocked-by, blocking, parent, sub-issues

### Relationships

- **`link(issue, target, relationship, target_repo?, repo?)`** — Create or remove issue relationships
  - Relationships: `blocked_by`, `blocks`, `parent_of`, `child_of`
  - Prefix with `-` to remove (e.g. `-blocked_by`)

### Admin

- **`audit(fix?, repo?)`** — Check workflow compliance: auth, repo settings, branch protection, stale branches, worktrees. `fix=true` auto-fixes safe issues.
- **`setup(repo?)`** — Configure repo: base + release branches, branch protection, auto-delete, auto-merge
- **`worktrees(cwd?)`** — List all worktrees with branch and issue number

## Auto-Detection

Most parameters are optional. Loom detects:
- **repo** from `git remote get-url origin`
- **issue number** from branch name (`feat/42` → 42)
- **branch type** from branch name (`feat/42` → feat)

Explicit parameters always override detected values.

## Commit Convention

Commit messages follow: `<type>: <short description>`

Types: `feat`, `fix`, `refactor`, `doc`, `test`, `chore`, `build`, `ci`, `perf`

## Typical Flow

1. `create_issue(title, body)` → creates issue
2. `start(issue)` → creates branch, pushes
3. `commit(message)` → stages, commits, pushes, auto-creates draft PR
4. _(repeat commits as needed)_
5. `finish()` → readies PR, enables auto-merge, cleans up locally
