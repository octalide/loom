package detect

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/octalide/loom/internal/config"
	"github.com/octalide/loom/internal/git"
)

type Context struct {
	Repo         string
	Owner        string
	IssueNumber  int
	BranchName   string
	BranchType   string
	Cwd          string
	IsWorktree   bool
	MainWorktree string
	Config       *config.Config
}

var branchRe = regexp.MustCompile(`^(\w+)/(\d+)$`)

func Detect(gitClient *git.Client, cwd string) *Context {
	ctx := &Context{Cwd: cwd}

	if cwd == "" {
		cwd, _ = os.Getwd()
		ctx.Cwd = cwd
	}

	// Find git root and load config
	root, err := gitClient.Root(cwd)
	if err != nil {
		ctx.Config = config.Default()
		return ctx
	}
	ctx.Config = config.Load(root)

	// Repo from config override or git remote
	if ctx.Config.Repo != "" {
		ctx.Repo = ctx.Config.Repo
	} else {
		remoteURL, err := gitClient.RemoteURL(cwd)
		if err == nil {
			ctx.Repo = parseRepoFromRemote(remoteURL)
		}
	}

	if ctx.Repo != "" {
		if idx := strings.Index(ctx.Repo, "/"); idx > 0 {
			ctx.Owner = ctx.Repo[:idx]
		}
	}

	// Current branch → issue number + type
	branch, err := gitClient.CurrentBranch(cwd)
	if err == nil {
		ctx.BranchName = branch
		if m := branchRe.FindStringSubmatch(branch); m != nil {
			ctx.BranchType = m[1]
			ctx.IssueNumber, _ = strconv.Atoi(m[2])
		}
	}

	// Worktree detection
	ctx.IsWorktree = gitClient.IsWorktree(cwd)
	if ctx.IsWorktree {
		ctx.MainWorktree, _ = gitClient.MainWorktree(cwd)
	}

	return ctx
}

func parseRepoFromRemote(url string) string {
	// SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(url, "git@") {
		url = strings.TrimPrefix(url, "git@github.com:")
		url = strings.TrimSuffix(url, ".git")
		return url
	}
	// HTTPS: https://github.com/owner/repo.git
	if strings.Contains(url, "github.com/") {
		parts := strings.Split(url, "github.com/")
		if len(parts) == 2 {
			repo := strings.TrimSuffix(parts[1], ".git")
			repo = strings.TrimSuffix(repo, "/")
			return repo
		}
	}
	return ""
}

// OwnerOf extracts the owner from an "owner/repo" string.
func OwnerOf(repo string) string {
	if idx := strings.Index(repo, "/"); idx > 0 {
		return repo[:idx]
	}
	return ""
}

// FirstNonEmpty returns the first non-empty string.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// FirstNonZero returns the first non-zero int.
func FirstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
