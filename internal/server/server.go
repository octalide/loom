package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/git"
)

var version = "dev"

type Server struct {
	mcp *mcp.Server
	git *git.Client
}

func New() (*Server, error) {
	s := &Server{
		mcp: mcp.NewServer(
			&mcp.Implementation{Name: "loom", Version: version},
			&mcp.ServerOptions{
				Instructions: "GitHub workflow automation: issues, branches, PRs, project boards.",
			},
		),
		git: git.New(),
	}

	s.registerTools()
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

type statusInput struct{}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "status",
		Description: "Get current workflow status: branch, uncommitted changes. Auto-detects from git state and .github/loom.yml.",
	}, s.handleStatus)
}

func (s *Server) handleStatus(ctx context.Context, req *mcp.CallToolRequest, in statusInput) (*mcp.CallToolResult, any, error) {
	cwd := ""

	branch, err := s.git.CurrentBranch(cwd)
	if err != nil {
		return errorResult("could not determine current branch: %v", err), nil, nil
	}

	b := newBuilder()
	b.Header("Status")
	b.KV("Branch", branch)

	if s.git.HasUncommittedChanges(cwd) {
		b.Warn("Uncommitted changes")
	} else {
		b.OK("Working tree clean")
	}

	return builderResult(b), nil, nil
}
