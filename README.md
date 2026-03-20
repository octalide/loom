# loom

Opinionated GitHub workflow MCP server. Enforces: issue → branch → PR → auto-merge.

Built for [Claude Code](https://claude.com/claude-code) and any MCP-compatible client.

## Prerequisites

- **Go 1.25+** — for installation
- **git** — for local branch/commit/push operations
- **gh** (GitHub CLI) — for auth token resolution. Run `gh auth login` once, and loom picks up your credentials automatically. Alternatively, set `GH_TOKEN` or `GITHUB_TOKEN`.

## Install

```sh
go install github.com/octalide/loom/cmd/loom@latest
claude mcp add --scope user --transport stdio loom -- loom
```

Or download a binary from [releases](https://github.com/octalide/loom/releases) and add it manually:

```sh
claude mcp add --scope user --transport stdio loom -- /path/to/loom
```

## Repo Configuration

Optional. Place `.github/loom.yml` in your repo:

```yaml
branches:
  base: "dev"           # base branch (default: dev)
  release: "main"       # release branch (default: main)

merge_method: "merge"   # merge, squash, or rebase (default: merge)

checks:                 # CI checks required for branch protection
  - build
  - test
```

## Usage

Once installed, loom's tools are available to your AI agent automatically. The agent can call `usage` for full documentation on tools, config, and conventions.

## License

MIT
