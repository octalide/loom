package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/shurcooL/githubv4"
)

type ProjectItem struct {
	Title  string
	Number int
	URL    string
	Status string
	Repo   string
}

func (c *Client) CreateProject(ctx context.Context, title, owner string) (number int, url string, err error) {
	ownerID, err := c.resolveOwnerID(ctx, owner)
	if err != nil {
		return 0, "", err
	}

	var mutation struct {
		CreateProjectV2 struct {
			ProjectV2 struct {
				Number int
				URL    string
			}
		} `graphql:"createProjectV2(input: $input)"`
	}
	type CreateProjectV2Input struct {
		OwnerID string `json:"ownerId"`
		Title   string `json:"title"`
	}
	err = c.GraphQL.Mutate(ctx, &mutation, CreateProjectV2Input{
		OwnerID: ownerID,
		Title:   title,
	}, nil)
	if err != nil {
		return 0, "", fmt.Errorf("create project: %w", err)
	}
	return mutation.CreateProjectV2.ProjectV2.Number, mutation.CreateProjectV2.ProjectV2.URL, nil
}

func (c *Client) AddIssueToProject(ctx context.Context, owner string, projectNumber int, issueURL string) (itemID string, err error) {
	info, err := c.discoverProject(ctx, owner, projectNumber)
	if err != nil {
		return "", err
	}

	contentID, err := c.nodeIDFromURL(ctx, issueURL)
	if err != nil {
		return "", fmt.Errorf("resolve issue node ID: %w", err)
	}

	var mutation struct {
		AddProjectV2ItemById struct {
			Item struct {
				ID string
			}
		} `graphql:"addProjectV2ItemById(input: $input)"`
	}
	type AddProjectV2ItemByIdInput struct {
		ProjectID string `json:"projectId"`
		ContentID string `json:"contentId"`
	}
	err = c.GraphQL.Mutate(ctx, &mutation, AddProjectV2ItemByIdInput{
		ProjectID: info.ProjectID,
		ContentID: contentID,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("add issue to project: %w", err)
	}
	return mutation.AddProjectV2ItemById.Item.ID, nil
}

func (c *Client) SetProjectStatus(ctx context.Context, owner string, projectNumber int, issueURL, status string, itemID string) error {
	info, err := c.discoverProject(ctx, owner, projectNumber)
	if err != nil {
		return err
	}

	optionID, ok := info.Options[status]
	if !ok {
		available := make([]string, 0, len(info.Options))
		for k := range info.Options {
			available = append(available, k)
		}
		return fmt.Errorf("status %q not found; available: %s", status, strings.Join(available, ", "))
	}

	if itemID == "" {
		itemID, err = c.findProjectItemID(ctx, owner, projectNumber, issueURL)
		if err != nil {
			return fmt.Errorf("find project item: %w", err)
		}
	}

	var mutation struct {
		UpdateProjectV2ItemFieldValue struct {
			ClientMutationID string
		} `graphql:"updateProjectV2ItemFieldValue(input: $input)"`
	}
	type FieldValue struct {
		SingleSelectOptionID string `json:"singleSelectOptionId"`
	}
	type UpdateProjectV2ItemFieldValueInput struct {
		ProjectID string     `json:"projectId"`
		ItemID    string     `json:"itemId"`
		FieldID   string     `json:"fieldId"`
		Value     FieldValue `json:"value"`
	}
	err = c.GraphQL.Mutate(ctx, &mutation, UpdateProjectV2ItemFieldValueInput{
		ProjectID: info.ProjectID,
		ItemID:    itemID,
		FieldID:   info.StatusFieldID,
		Value:     FieldValue{SingleSelectOptionID: optionID},
	}, nil)
	if err != nil {
		return fmt.Errorf("set project status: %w", err)
	}
	return nil
}

func (c *Client) ArchiveProjectItem(ctx context.Context, owner string, projectNumber int, issueURL string) error {
	info, err := c.discoverProject(ctx, owner, projectNumber)
	if err != nil {
		return err
	}

	itemID, err := c.findProjectItemID(ctx, owner, projectNumber, issueURL)
	if err != nil {
		return fmt.Errorf("find project item: %w", err)
	}

	var mutation struct {
		ArchiveProjectV2Item struct {
			ClientMutationID string
		} `graphql:"archiveProjectV2Item(input: $input)"`
	}
	type ArchiveProjectV2ItemInput struct {
		ProjectID string `json:"projectId"`
		ItemID    string `json:"itemId"`
	}
	err = c.GraphQL.Mutate(ctx, &mutation, ArchiveProjectV2ItemInput{
		ProjectID: info.ProjectID,
		ItemID:    itemID,
	}, nil)
	if err != nil {
		return fmt.Errorf("archive project item: %w", err)
	}
	return nil
}

func (c *Client) GetProjectItems(ctx context.Context, owner string, projectNumber int) (map[string][]ProjectItem, error) {
	var query struct {
		User struct {
			ProjectV2 struct {
				Items struct {
					Nodes []struct {
						FieldValueByName struct {
							ProjectV2ItemFieldSingleSelectValue struct {
								Name string
							} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
						} `graphql:"fieldValueByName(name: \"Status\")"`
						Content struct {
							Issue struct {
								Number     int
								Title      string
								URL        string
								Repository struct {
									NameWithOwner string
								}
							} `graphql:"... on Issue"`
							PullRequest struct {
								Number     int
								Title      string
								URL        string
								Repository struct {
									NameWithOwner string
								}
							} `graphql:"... on PullRequest"`
						}
					}
				} `graphql:"items(first: 100)"`
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"user(login: $owner)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(projectNumber),
	}

	err := c.GraphQL.Query(ctx, &query, vars)
	if err != nil {
		// Try as organization
		return c.getOrgProjectItems(ctx, owner, projectNumber)
	}

	grouped := make(map[string][]ProjectItem)
	for _, item := range query.User.ProjectV2.Items.Nodes {
		status := item.FieldValueByName.ProjectV2ItemFieldSingleSelectValue.Name
		if status == "" {
			status = "No Status"
		}

		var pi ProjectItem
		if item.Content.Issue.Number != 0 {
			pi = ProjectItem{
				Title:  item.Content.Issue.Title,
				Number: item.Content.Issue.Number,
				URL:    item.Content.Issue.URL,
				Repo:   item.Content.Issue.Repository.NameWithOwner,
				Status: status,
			}
		} else if item.Content.PullRequest.Number != 0 {
			pi = ProjectItem{
				Title:  item.Content.PullRequest.Title,
				Number: item.Content.PullRequest.Number,
				URL:    item.Content.PullRequest.URL,
				Repo:   item.Content.PullRequest.Repository.NameWithOwner,
				Status: status,
			}
		} else {
			continue
		}
		grouped[status] = append(grouped[status], pi)
	}
	return grouped, nil
}

func (c *Client) getOrgProjectItems(ctx context.Context, owner string, projectNumber int) (map[string][]ProjectItem, error) {
	var query struct {
		Organization struct {
			ProjectV2 struct {
				Items struct {
					Nodes []struct {
						FieldValueByName struct {
							ProjectV2ItemFieldSingleSelectValue struct {
								Name string
							} `graphql:"... on ProjectV2ItemFieldSingleSelectValue"`
						} `graphql:"fieldValueByName(name: \"Status\")"`
						Content struct {
							Issue struct {
								Number     int
								Title      string
								URL        string
								Repository struct {
									NameWithOwner string
								}
							} `graphql:"... on Issue"`
						}
					}
				} `graphql:"items(first: 100)"`
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"organization(login: $owner)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(projectNumber),
	}

	err := c.GraphQL.Query(ctx, &query, vars)
	if err != nil {
		return nil, fmt.Errorf("get project items: %w", err)
	}

	grouped := make(map[string][]ProjectItem)
	for _, item := range query.Organization.ProjectV2.Items.Nodes {
		status := item.FieldValueByName.ProjectV2ItemFieldSingleSelectValue.Name
		if status == "" {
			status = "No Status"
		}
		if item.Content.Issue.Number == 0 {
			continue
		}
		grouped[status] = append(grouped[status], ProjectItem{
			Title:  item.Content.Issue.Title,
			Number: item.Content.Issue.Number,
			URL:    item.Content.Issue.URL,
			Repo:   item.Content.Issue.Repository.NameWithOwner,
			Status: status,
		})
	}
	return grouped, nil
}

// discoverProject finds project ID, status field ID, and status option IDs.
// Results are cached per-session.
func (c *Client) discoverProject(ctx context.Context, owner string, projectNumber int) (*projectInfo, error) {
	key := fmt.Sprintf("%s/%d", owner, projectNumber)
	if cached, ok := c.projectCache.Load(key); ok {
		return cached.(*projectInfo), nil
	}

	// Try as user first, then as org
	info, err := c.discoverUserProject(ctx, owner, projectNumber)
	if err != nil {
		info, err = c.discoverOrgProject(ctx, owner, projectNumber)
		if err != nil {
			return nil, fmt.Errorf("discover project #%d for %s: %w", projectNumber, owner, err)
		}
	}

	c.projectCache.Store(key, info)
	return info, nil
}

func (c *Client) discoverUserProject(ctx context.Context, owner string, projectNumber int) (*projectInfo, error) {
	var query struct {
		User struct {
			ProjectV2 struct {
				ID    string
				Field struct {
					ProjectV2SingleSelectField struct {
						ID      string
						Options []struct {
							ID   string
							Name string
						}
					} `graphql:"... on ProjectV2SingleSelectField"`
				} `graphql:"field(name: \"Status\")"`
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"user(login: $owner)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(projectNumber),
	}

	if err := c.GraphQL.Query(ctx, &query, vars); err != nil {
		return nil, err
	}

	field := query.User.ProjectV2.Field.ProjectV2SingleSelectField
	if field.ID == "" {
		return nil, fmt.Errorf("Status field not found")
	}

	options := make(map[string]string)
	for _, opt := range field.Options {
		options[opt.Name] = opt.ID
	}

	return &projectInfo{
		ProjectID:     query.User.ProjectV2.ID,
		StatusFieldID: field.ID,
		Options:       options,
	}, nil
}

func (c *Client) discoverOrgProject(ctx context.Context, owner string, projectNumber int) (*projectInfo, error) {
	var query struct {
		Organization struct {
			ProjectV2 struct {
				ID    string
				Field struct {
					ProjectV2SingleSelectField struct {
						ID      string
						Options []struct {
							ID   string
							Name string
						}
					} `graphql:"... on ProjectV2SingleSelectField"`
				} `graphql:"field(name: \"Status\")"`
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"organization(login: $owner)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(projectNumber),
	}

	if err := c.GraphQL.Query(ctx, &query, vars); err != nil {
		return nil, err
	}

	field := query.Organization.ProjectV2.Field.ProjectV2SingleSelectField
	if field.ID == "" {
		return nil, fmt.Errorf("Status field not found")
	}

	options := make(map[string]string)
	for _, opt := range field.Options {
		options[opt.Name] = opt.ID
	}

	return &projectInfo{
		ProjectID:     query.Organization.ProjectV2.ID,
		StatusFieldID: field.ID,
		Options:       options,
	}, nil
}

func (c *Client) findProjectItemID(ctx context.Context, owner string, projectNumber int, issueURL string) (string, error) {
	// Search through project items for the one matching this issue URL
	var query struct {
		User struct {
			ProjectV2 struct {
				Items struct {
					Nodes []struct {
						ID      string
						Content struct {
							Issue struct {
								URL string
							} `graphql:"... on Issue"`
						}
					}
				} `graphql:"items(first: 100)"`
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"user(login: $owner)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(projectNumber),
	}

	err := c.GraphQL.Query(ctx, &query, vars)
	if err != nil {
		// Try org
		return c.findOrgProjectItemID(ctx, owner, projectNumber, issueURL)
	}

	for _, item := range query.User.ProjectV2.Items.Nodes {
		if item.Content.Issue.URL == issueURL {
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("issue not found on project #%d", projectNumber)
}

func (c *Client) findOrgProjectItemID(ctx context.Context, owner string, projectNumber int, issueURL string) (string, error) {
	var query struct {
		Organization struct {
			ProjectV2 struct {
				Items struct {
					Nodes []struct {
						ID      string
						Content struct {
							Issue struct {
								URL string
							} `graphql:"... on Issue"`
						}
					}
				} `graphql:"items(first: 100)"`
			} `graphql:"projectV2(number: $number)"`
		} `graphql:"organization(login: $owner)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"number": githubv4.Int(projectNumber),
	}

	if err := c.GraphQL.Query(ctx, &query, vars); err != nil {
		return "", err
	}

	for _, item := range query.Organization.ProjectV2.Items.Nodes {
		if item.Content.Issue.URL == issueURL {
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("issue not found on project #%d", projectNumber)
}

func (c *Client) resolveOwnerID(ctx context.Context, owner string) (string, error) {
	// Try user first
	var userQuery struct {
		User struct {
			ID string
		} `graphql:"user(login: $login)"`
	}
	vars := map[string]interface{}{"login": githubv4.String(owner)}
	if err := c.GraphQL.Query(ctx, &userQuery, vars); err == nil && userQuery.User.ID != "" {
		return userQuery.User.ID, nil
	}

	// Try org
	var orgQuery struct {
		Organization struct {
			ID string
		} `graphql:"organization(login: $login)"`
	}
	if err := c.GraphQL.Query(ctx, &orgQuery, vars); err == nil && orgQuery.Organization.ID != "" {
		return orgQuery.Organization.ID, nil
	}

	return "", fmt.Errorf("could not resolve owner %q", owner)
}

func (c *Client) nodeIDFromURL(ctx context.Context, issueURL string) (string, error) {
	// Parse issue URL: https://github.com/owner/repo/issues/42
	parts := strings.Split(issueURL, "/")
	if len(parts) < 4 {
		return "", fmt.Errorf("invalid issue URL: %s", issueURL)
	}
	// Extract owner/repo and issue number from URL
	repo := parts[len(parts)-4] + "/" + parts[len(parts)-3]
	var issueNum int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &issueNum); err != nil {
		return "", fmt.Errorf("invalid issue number in URL %s: %w", issueURL, err)
	}
	return c.issueNodeID(ctx, repo, issueNum)
}
