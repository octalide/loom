# loom

Loom is an MCP server that gives AI agents a structured way to work with GitHub. It encodes a specific workflow model — issue-driven development with convention-enforced branching, commits, and merges — so that agents can participate in real codebases without improvising their way through `git` and `gh` commands.

## Philosophy

Most AI coding tools treat GitHub as an afterthought: generate some code, maybe commit it, hope the branch name makes sense. Loom takes the opposite approach. It starts from the workflow and works backward to the tools.

The model is simple and consistent by design:

**Every change starts with an issue.** Issues are the unit of work. They get labels, they get tracked, they get closed when the work merges.

**Every issue gets exactly one branch and one PR.** Branch names are derived from the issue (`feat/42`, `fix/17`). PRs reference their issue (`Closes #42`). One issue, one branch, one PR, no ambiguity about what's in flight.

**Merging works with your CI, not around it.** By default, `finish` marks a PR ready and enables auto-merge — CI decides when it ships. If the repo doesn't use auto-merge, or if checks aren't configured, loom merges directly when the PR is clean. If CI is failing, it tells you. The point is that loom adapts to whatever CI pipeline you have (or don't have) rather than imposing its own.

**The repo configures itself.** Running `setup` on a new repo applies branch protection, enables auto-delete and auto-merge, creates convention-aligned labels, and walks you through creating the config file. The tool sets up the infrastructure and the config follows.

This consistency is the feature. When the workflow is predictable, a project stays internally coherent — the same conventions from the first commit to the hundredth, whether a human or an agent is doing the work.

Loom is the default path, not a cage. It's still a git repo. Hotfix branches, cherry-picks, manual merges, direct pushes — nothing is disabled. When the standard workflow fits, loom handles it. When it doesn't, you step outside and do what you need to do.

## Install

```sh
go install github.com/octalide/loom/cmd/loom@latest
```

Or grab a binary from [releases](https://github.com/octalide/loom/releases).

Then register it as an MCP server:

```sh
claude mcp add --scope user --transport stdio loom -- loom
```

Loom resolves GitHub credentials from `GH_TOKEN`, `GITHUB_TOKEN`, or `gh auth login`.

## Setup

Point your agent at a repo and tell it to run `setup`. Loom will:

- Configure repo settings (auto-delete head branches, auto-merge)
- Set the default branch
- Apply branch protection rules
- Create convention-aligned labels (`feat`, `fix`, `refactor`, `doc`, etc.)
- Walk you through removing GitHub's default labels and adding project-specific ones
- Offer to create `.github/loom.yml` with the detected settings

The config file is optional — everything has sensible defaults. You only need it to override those defaults or declare required CI checks.

## Configuration

All configuration lives in `.github/loom.yml`. Every field is optional.

```yaml
branches:
  base: "dev"                              # default: dev
  release: "main"                          # default: main
  types: [feat, fix, doc, refactor, issue] # allowed branch prefixes

merge_method: "merge"   # merge, squash, or rebase (default: merge)
strict: false           # require branches up-to-date before merge (default: false)

checks:                 # CI status checks required for branch protection
  - build
  - test
```

**Branches** control where feature branches originate and where releases land. `types` defines the allowed branch prefixes — `start` will reject anything not in this list.

**Merge method** determines how PRs are merged. All three GitHub merge strategies are supported.

**Strict mode** mirrors GitHub's "require branches to be up to date" setting. When enabled, PRs must be rebased on the latest base branch before merging.

**Checks** are passed to branch protection rules during `setup`. They tie loom's workflow to your CI — auto-merge won't proceed until these checks pass.

## Workflow

The agent lifecycle for a single change:

```
create_issue  ->  start  ->  commit (repeat)  ->  finish
```

`start` creates a branch from base and pushes it. `commit` stages, commits, pushes, and auto-creates a draft PR on first push. `finish` marks the PR ready, enables auto-merge if available, and cleans up the local branch. After that, GitHub handles the rest — CI runs, the PR merges when checks pass, and the issue closes automatically.

Agents can post notes to issues with `comment` (progress updates, decisions, implementation context) and check their current work state with `status`. For broader repo health, `audit` checks everything from branch protection to stale PRs to orphaned branches, and can auto-fix most issues.

The full tool reference is built into the server — agents can call `usage` at any time for complete documentation on tools, config, and conventions.

## License

MIT
