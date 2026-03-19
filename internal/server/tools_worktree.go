package server

import (
	"context"
	"fmt"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type worktreesInput struct {
	Cwd string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleWorktrees(ctx context.Context, req *mcp.CallToolRequest, in worktreesInput) (*mcp.CallToolResult, any, error) {
	dc := s.detect(in.Cwd)

	worktrees, err := s.git.ListWorktrees(dc.Cwd)
	if err != nil || len(worktrees) == 0 {
		return infoResult("No worktrees found (not a git repo, or only the main working tree)."), nil, nil
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("Worktrees (%d)", len(worktrees)))

	issueRe := regexp.MustCompile(`/(\d+)$`)

	for i, wt := range worktrees {
		label := ""
		if i == 0 {
			label = "(main)"
		}
		if wt.Bare {
			label = "(bare)"
		}
		if wt.Detached {
			label = "(detached)"
		}

		issueTag := ""
		if wt.Branch != "" {
			if m := issueRe.FindStringSubmatch(wt.Branch); m != nil {
				issueTag = fmt.Sprintf(" issue #%s", m[1])
			}
		}

		branchInfo := "[no branch]"
		if wt.Branch != "" {
			branchInfo = "[" + wt.Branch + "]"
		}

		text := fmt.Sprintf("%s %s%s %s", wt.Path, branchInfo, issueTag, label)
		b.Bullet(text)
	}

	return builderResult(b), nil, nil
}
