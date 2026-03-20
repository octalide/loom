package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/octalide/loom/internal/detect"
	gh "github.com/octalide/loom/internal/github"
	"github.com/octalide/loom/internal/output"
)

type statusInput struct {
	Repo    string `json:"repo,omitempty" jsonschema:"Repository in owner/repo format. Auto-detected if omitted."`
	Project string `json:"project,omitempty" jsonschema:"GitHub Project number. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleStatus(ctx context.Context, req *mcp.CallToolRequest, in statusInput) (*mcp.CallToolResult, any, error) {
	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	owner := detect.FirstNonEmpty(dc.Owner, detect.OwnerOf(repo))

	b := newBuilder()
	b.Header(fmt.Sprintf("Status: %s", detect.FirstNonEmpty(repo, "(unknown repo)")))

	b.KV("Branch", detect.FirstNonEmpty(dc.BranchName, "(unknown)"))

	if s.git.HasUncommittedChanges(dc.Cwd) {
		b.Warn("Uncommitted changes")
	} else {
		b.OK("Working tree clean")
	}

	if s.gh != nil && repo != "" {
		prs, err := s.gh.ListOpenPRs(ctx, repo)
		if err == nil {
			b.Section(fmt.Sprintf("Open PRs (%d)", len(prs)))
			if len(prs) == 0 {
				b.Text("None")
			}
			for _, pr := range prs {
				b.Bullet(fmt.Sprintf("#%d [%s] %s", pr.Number, pr.HeadRefName, pr.Title))
			}

			var alerts []string
			now := time.Now()
			branchPattern := regexp.MustCompile(`^(\w+)/(\d+)`)
			for _, pr := range prs {
				createdAt, _ := time.Parse("2006-01-02T15:04:05Z", pr.CreatedAt)
				updatedAt, _ := time.Parse("2006-01-02T15:04:05Z", pr.UpdatedAt)
				age := now.Sub(createdAt)
				idle := now.Sub(updatedAt)

				if pr.IsDraft && age.Hours() > 7*24 {
					alerts = append(alerts, fmt.Sprintf("PR #%d: draft for %d days", pr.Number, int(age.Hours()/24)))
				} else if !pr.IsDraft && idle.Hours() > 7*24 {
					alerts = append(alerts, fmt.Sprintf("PR #%d: idle for %d days", pr.Number, int(idle.Hours()/24)))
				}

				if !containsCloseRef(pr.Body, branchPattern, pr.HeadRefName) {
					alerts = append(alerts, fmt.Sprintf("PR #%d: missing Closes #N", pr.Number))
				}

				readiness, err := s.gh.AssessMergeReadiness(ctx, repo, pr.Number)
				if err == nil {
					for _, check := range readiness.Checks {
						if check.Conclusion == "failure" || check.Conclusion == "error" {
							alerts = append(alerts, fmt.Sprintf("PR #%d: failing CI", pr.Number))
							break
						}
					}
				}
			}
			if len(alerts) > 0 {
				b.Section("Attention Needed")
				for _, a := range alerts {
					b.Warn("%s", a)
				}
			}
		}

		project := detect.FirstNonZero(parseInt(in.Project), dc.Project)
		if project != 0 {
			items, err := s.gh.GetProjectItems(ctx, owner, project)
			if err == nil {
				b.Section(fmt.Sprintf("Project #%d", project))
				for status, entries := range items {
					b.Text(fmt.Sprintf("**%s**:", status))
					for _, e := range entries {
						repoTag := ""
						if e.Repo != "" && e.Repo != repo {
							repoTag = fmt.Sprintf(" [%s]", e.Repo)
						}
						b.Bullet(fmt.Sprintf("#%d%s %s", e.Number, repoTag, e.Title))
					}
				}
			} else {
				b.Warn("failed to fetch project board: %v", err)
			}
		}
	}

	return builderResult(b), nil, nil
}

type contextInput struct {
	Issue   string `json:"issue" jsonschema:"Issue number"`
	Repo    string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Project string `json:"project,omitempty" jsonschema:"Project number. Auto-detected if omitted."`
	Cwd     string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleContext(ctx context.Context, req *mcp.CallToolRequest, in contextInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	issueNum := parseInt(in.Issue)
	ghIssue, err := s.gh.GetIssue(ctx, repo, issueNum)
	if err != nil {
		return errorResult("failed to fetch issue #%d: %v", issueNum, err), nil, nil
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("Issue #%d: %s", ghIssue.Number, ghIssue.Title))
	b.KV("Repo", repo)
	b.KV("State", ghIssue.State)
	b.KV("URL", ghIssue.URL)
	if len(ghIssue.Labels) > 0 {
		b.KV("Labels", strings.Join(ghIssue.Labels, ", "))
	}
	if len(ghIssue.Assignees) > 0 {
		b.KV("Assignees", strings.Join(ghIssue.Assignees, ", "))
	}

	b.Section("Description")
	b.Text(output.CleanBody(ghIssue.Body))

	comments, err := s.gh.GetIssueComments(ctx, repo, issueNum)
	b.Section(fmt.Sprintf("Comments (%d)", len(comments)))
	if err == nil && len(comments) > 0 {
		for _, c := range comments {
			b.Text(fmt.Sprintf("\n@%s (%s):\n%s", c.Author, c.CreatedAt, output.CleanBody(c.Body)))
		}
	} else {
		b.Text("(none)")
	}

	// Linked PR
	pr, err := s.gh.FindPRForIssue(ctx, repo, issueNum)
	if err == nil {
		b.Section("Linked PR")
		b.KV("PR", fmt.Sprintf("#%d: %s", pr.Number, pr.Title))
		b.KV("Branch", pr.HeadRefName)
		b.KV("State", pr.State)
		if pr.IsDraft {
			b.KV("Draft", "yes")
		}

		readiness, err := s.gh.AssessMergeReadiness(ctx, repo, pr.Number)
		if err == nil {
			b.Section("PR Readiness")
			for _, line := range readiness.Summary {
				b.Bullet(line)
			}
		}
	}

	// Dependencies
	deps, err := s.gh.GetDependencies(ctx, repo, issueNum)
	if err == nil {
		hasDeps := len(deps.BlockedBy) > 0 || len(deps.Blocking) > 0 || deps.Parent != nil || len(deps.SubIssues) > 0
		if hasDeps {
			b.Section("Dependencies")
			if len(deps.BlockedBy) > 0 {
				b.Text("**Blocked by**:")
				for _, d := range deps.BlockedBy {
					b.Bullet(formatDep(d, repo))
				}
			}
			if len(deps.Blocking) > 0 {
				b.Text("**Blocking**:")
				for _, d := range deps.Blocking {
					b.Bullet(formatDep(d, repo))
				}
			}
			if deps.Parent != nil {
				b.Text("**Parent**:")
				b.Bullet(formatDep(*deps.Parent, repo))
			}
			if len(deps.SubIssues) > 0 {
				b.Text("**Sub-issues**:")
				for _, d := range deps.SubIssues {
					b.Bullet(formatDep(d, repo))
				}
			}
		}
	}

	return builderResult(b), nil, nil
}

type activityInput struct {
	Issue string `json:"issue" jsonschema:"Issue number"`
	Since string `json:"since,omitempty" jsonschema:"ISO 8601 timestamp to filter activity"`
	Repo  string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd   string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleActivity(ctx context.Context, req *mcp.CallToolRequest, in activityInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	issueNum := parseInt(in.Issue)
	activity, err := s.gh.GetActivity(ctx, repo, issueNum, in.Since)
	if err != nil {
		return errorResult("failed to fetch activity: %v", err), nil, nil
	}

	b := newBuilder()
	title := fmt.Sprintf("Activity: issue #%d (%s)", issueNum, repo)
	if in.Since != "" {
		title += " since " + in.Since
	}
	b.Header(title)

	hasActivity := false

	if len(activity.IssueComments) > 0 {
		hasActivity = true
		b.Section(fmt.Sprintf("Issue Comments (%d)", len(activity.IssueComments)))
		for _, c := range activity.IssueComments {
			b.Text(fmt.Sprintf("\n@%s (%s):\n%s", c.Author, c.CreatedAt, output.CleanBody(c.Body)))
		}
	}

	if activity.LinkedPR != nil {
		b.Info("Linked PR: #%d (%s)", activity.LinkedPR.Number, activity.LinkedPR.State)
	} else {
		b.Info("No linked PR found")
		if !hasActivity {
			b.Text("No new activity")
		}
		return builderResult(b), nil, nil
	}

	if len(activity.PRComments) > 0 {
		hasActivity = true
		b.Section(fmt.Sprintf("PR Comments (%d)", len(activity.PRComments)))
		for _, c := range activity.PRComments {
			b.Text(fmt.Sprintf("\n@%s (%s):\n%s", c.Author, c.CreatedAt, output.CleanBody(c.Body)))
		}
	}

	if len(activity.PRReviews) > 0 {
		hasActivity = true
		b.Section(fmt.Sprintf("Reviews (%d)", len(activity.PRReviews)))
		for _, r := range activity.PRReviews {
			b.Text(fmt.Sprintf("@%s: %s (%s)", r.Author, r.State, r.SubmittedAt))
			body := output.CleanBody(r.Body)
			if body != "(empty)" {
				b.Text("  " + body)
			}
		}
	}

	if len(activity.PRReviewComments) > 0 {
		hasActivity = true
		total := 0
		for _, cs := range activity.PRReviewComments {
			total += len(cs)
		}
		b.Section(fmt.Sprintf("Inline Comments (%d)", total))
		for path, comments := range activity.PRReviewComments {
			b.Text(fmt.Sprintf("\n**%s**:", path))
			for _, c := range comments {
				lineInfo := ""
				if c.Line > 0 {
					lineInfo = fmt.Sprintf("L%d ", c.Line)
				}
				b.Bullet(fmt.Sprintf("%s@%s: %s", lineInfo, c.Author, c.Body))
			}
		}
	}

	if !hasActivity {
		b.Text("\nNo new activity")
	}

	return builderResult(b), nil, nil
}

type prFeedbackInput struct {
	PR   string `json:"pr" jsonschema:"PR number"`
	Repo string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd  string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handlePRFeedback(ctx context.Context, req *mcp.CallToolRequest, in prFeedbackInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	prNum := parseInt(in.PR)
	pr, err := s.gh.GetPR(ctx, repo, prNum)
	if err != nil {
		return errorResult("failed to fetch PR #%d: %v", prNum, err), nil, nil
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("PR #%d: %s", pr.Number, pr.Title))
	b.KV("Repo", repo)
	b.KV("Branch", fmt.Sprintf("%s → %s", pr.HeadRefName, pr.BaseRefName))
	b.KV("Changes", fmt.Sprintf("+%d -%d across %d files", pr.Additions, pr.Deletions, pr.ChangedFiles))

	// Reviews
	reviews, err := s.gh.GetPRReviews(ctx, repo, prNum)
	b.Section(fmt.Sprintf("Reviews (%d)", len(reviews)))
	if err == nil && len(reviews) > 0 {
		for _, r := range reviews {
			b.Text(fmt.Sprintf("@%s: %s (%s)", r.Author, r.State, r.SubmittedAt))
			body := output.CleanBody(r.Body)
			if body != "(empty)" {
				b.Text("  " + body)
			}
		}
	} else {
		b.Text("(none)")
	}

	// PR comments
	prComments, err := s.gh.GetPRComments(ctx, repo, prNum)
	if err == nil && len(prComments) > 0 {
		b.Section(fmt.Sprintf("PR Comments (%d)", len(prComments)))
		for _, c := range prComments {
			b.Text(fmt.Sprintf("\n@%s (%s):\n%s", c.Author, c.CreatedAt, output.CleanBody(c.Body)))
		}
	}

	// Inline review comments
	byFile, err := s.gh.GetPRReviewComments(ctx, repo, prNum)
	if err == nil {
		total := 0
		for _, cs := range byFile {
			total += len(cs)
		}
		b.Section(fmt.Sprintf("Inline Comments (%d)", total))
		if total > 0 {
			for path, comments := range byFile {
				b.Text(fmt.Sprintf("\n**%s**:", path))
				for _, c := range comments {
					lineInfo := ""
					if c.Line > 0 {
						lineInfo = fmt.Sprintf("L%d ", c.Line)
					}
					reply := ""
					if c.InReplyTo != 0 {
						reply = " (reply)"
					}
					b.Bullet(fmt.Sprintf("%s@%s%s: %s", lineInfo, c.Author, reply, c.Body))
				}
			}
		} else {
			b.Text("(none)")
		}
	}

	// Merge readiness
	readiness, err := s.gh.AssessMergeReadiness(ctx, repo, prNum)
	if err == nil {
		b.Section("Merge Readiness")
		for _, line := range readiness.Summary {
			b.Bullet(line)
		}
	}

	return builderResult(b), nil, nil
}

type dependenciesInput struct {
	Issue string `json:"issue" jsonschema:"Issue number"`
	Repo  string `json:"repo,omitempty" jsonschema:"Repository. Auto-detected if omitted."`
	Cwd   string `json:"cwd,omitempty" jsonschema:"Working directory for git operations"`
}

func (s *Server) handleDependencies(ctx context.Context, req *mcp.CallToolRequest, in dependenciesInput) (*mcp.CallToolResult, any, error) {
	if r := s.requireGH(); r != nil {
		return r, nil, nil
	}

	dc := s.detect(in.Cwd)
	repo := detect.FirstNonEmpty(in.Repo, dc.Repo)
	if repo == "" {
		return errorResult("could not detect repo; pass explicitly"), nil, nil
	}

	issueNum := parseInt(in.Issue)
	deps, err := s.gh.GetDependencies(ctx, repo, issueNum)
	if err != nil {
		return errorResult("failed to fetch dependencies: %v", err), nil, nil
	}

	b := newBuilder()
	b.Header(fmt.Sprintf("Dependencies: #%d (%s)", issueNum, repo))

	hasAny := false

	if len(deps.BlockedBy) > 0 {
		hasAny = true
		b.Section(fmt.Sprintf("Blocked By (%d)", len(deps.BlockedBy)))
		for _, d := range deps.BlockedBy {
			b.Bullet(formatDep(d, repo))
		}
	}
	if len(deps.Blocking) > 0 {
		hasAny = true
		b.Section(fmt.Sprintf("Blocking (%d)", len(deps.Blocking)))
		for _, d := range deps.Blocking {
			b.Bullet(formatDep(d, repo))
		}
	}
	if deps.Parent != nil {
		hasAny = true
		b.Section("Parent")
		b.Bullet(formatDep(*deps.Parent, repo))
	}
	if len(deps.SubIssues) > 0 {
		hasAny = true
		b.Section(fmt.Sprintf("Sub-issues (%d)", len(deps.SubIssues)))
		for _, d := range deps.SubIssues {
			b.Bullet(formatDep(d, repo))
		}
	}

	if !hasAny {
		b.Info("No relationships found")
	}

	return builderResult(b), nil, nil
}

func formatDep(d gh.Dependency, contextRepo string) string {
	repoTag := ""
	if d.Repo != "" && d.Repo != contextRepo {
		repoTag = fmt.Sprintf(" [%s]", d.Repo)
	}
	stateTag := ""
	if strings.EqualFold(d.State, "CLOSED") || strings.EqualFold(d.State, "closed") {
		stateTag = " (closed)"
	}
	return fmt.Sprintf("#%d%s %s%s", d.Number, repoTag, d.Title, stateTag)
}
