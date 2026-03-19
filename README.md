# loom

Opinionated GitHub workflow MCP server. Enforces: issue → branch → PR → auto-merge, with GitHub Projects V2 board tracking.

Built for [Claude Code](https://claude.com/claude-code) and any MCP-compatible client.

## Install

Download a binary from [releases](https://github.com/octalide/loom/releases), or build from source:

```sh
go install github.com/octalide/loom/cmd/loom@latest
```

## Prerequisites

- **git** — for local branch/commit/push operations
- **gh** (GitHub CLI) — for auth token resolution. Run `gh auth login` once, and loom picks up your credentials automatically. Alternatively, set `GH_TOKEN` or `GITHUB_TOKEN`.

## Configure MCP Client

Add to your MCP configuration (e.g. `~/.mcp.json` for Claude Code):

```json
{
  "mcpServers": {
    "loom": {
      "type": "stdio",
      "command": "loom"
    }
  }
}
```

## Repo Configuration

Optional. Place `.github/loom.yml` in your repo:

```yaml
project: 5              # GitHub Projects V2 number
branches:
  base: "dev"           # base branch (default: dev)
  release: "main"       # release branch (default: main)
```

See [CLAUDE.md](CLAUDE.md) for full tool documentation.

## License

MIT
