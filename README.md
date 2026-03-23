# loom

Loom is an MCP server that gives AI agents a structured way to work with GitHub. It encodes a specific workflow model — issue-driven development with convention-enforced branching, commits, and merges — so that agents can participate in real codebases without improvising their way through `git` and `gh` commands.

## Philosophy

Most AI coding tools treat GitHub as an afterthought: generate some code, maybe commit it, hope the branch name makes sense. Loom takes the opposite approach. It starts from the workflow and works backward to the tools.

The model is simple and rigid by design:

**Every change starts with an issue.** Issues are the unit of work. They get labels, they get tracked, they get closed when the work merges. No drive-by commits.

**Every issue gets exactly one branch and one PR.** Branch names are derived from the issue (`feat/42`, `fix/17`). PRs reference their issue (`Closes #42`). This isn't configurable — it's the point. One issue, one branch, one PR, no ambiguity about what's in flight.

**PRs merge themselves.** Loom enables auto-merge by default. When an agent finishes work, the PR is marked ready, auto-merge is enabled, and CI decides when it ships. No manual merge buttons, no agents polling for green checks.

**The repo configures itself.** Running `setup` on a new repo applies branch protection, enables auto-delete and auto-merge, creates convention-aligned labels, and walks you through creating the config file. You don't write YAML first and hope it's right — the tool sets up the infrastructure and the config follows.

This rigidity is the feature. When the workflow is predictable, agents can operate autonomously across repos without per-project onboarding. A human or agent working on repo A follows the same cycle as repo B.

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

The config file is optional — everything has sensible defaults (`dev` as base branch, `main` as release branch, merge commits). You only need it to override those defaults or declare required CI checks.

```yaml
branches:
  base: "dev"
  release: "main"
  types: [feat, fix, doc, refactor, issue]

merge_method: "merge"   # merge, squash, or rebase

checks:                  # required CI status checks
  - build
  - test
```

## Workflow

The agent lifecycle for a single change:

```
create_issue  ->  start  ->  commit (repeat)  ->  finish
```

`start` creates a branch from base and pushes it. `commit` stages, commits, pushes, and auto-creates a draft PR on first push. `finish` marks the PR ready, enables auto-merge, and cleans up the local branch. After that, GitHub handles the rest — CI runs, PR merges when checks pass, issue closes automatically.

Agents can post notes to issues with `comment` (progress updates, decisions, implementation context) and check their current work state with `status`. For broader repo health, `audit` checks everything from branch protection to stale PRs to orphaned branches, and can auto-fix most issues.

The full tool reference is built into the server — agents can call `usage` at any time for complete documentation on tools, config, and conventions.

## License

MIT
