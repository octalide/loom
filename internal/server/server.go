package server

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/detect"
	"github.com/octalide/loom/internal/git"
	gh "github.com/octalide/loom/internal/github"
)

var version = "dev"

const detectCacheTTL = 5 * time.Second

type Server struct {
	mcp *mcp.Server
	git *git.Client
	gh  *gh.Client

	detectMu    sync.Mutex
	detectCache *detect.Context
	detectTime  time.Time
	detectCwd   string
}

func New() (*Server, error) {
	token, err := gh.ResolveToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loom: warning: %v\n", err)
	}

	var ghClient *gh.Client
	if token != "" {
		ghClient = gh.NewClient(token)
	}

	s := &Server{
		mcp: mcp.NewServer(
			&mcp.Implementation{Name: "loom", Version: version},
			&mcp.ServerOptions{
				Instructions: "GitHub workflow automation: issues, branches, PRs, releases. Call `usage` first if you are unfamiliar with loom's workflow conventions (branch naming, commit messages, PR lifecycle). All tools auto-detect repo, branch, and issue number from the local git state.",
			},
		),
		git: git.New(),
		gh:  ghClient,
	}

	s.registerTools()
	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) detect(cwd string) *detect.Context {
	s.detectMu.Lock()
	defer s.detectMu.Unlock()

	if s.detectCache != nil && s.detectCwd == cwd && time.Since(s.detectTime) < detectCacheTTL {
		return s.detectCache
	}

	ctx := detect.Detect(s.git, cwd)
	s.detectCache = ctx
	s.detectCwd = cwd
	s.detectTime = time.Now()
	return ctx
}

func (s *Server) invalidateDetectCache() {
	s.detectMu.Lock()
	s.detectCache = nil
	s.detectMu.Unlock()
}

func (s *Server) requireGH() *mcp.CallToolResult {
	if s.gh == nil {
		return errorResult("no GitHub token available; set GH_TOKEN, GITHUB_TOKEN, or run 'gh auth login'")
	}
	return nil
}
