package handlers

import (
	"fmt"
	"log"

	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ListProjects lists CloudStack projects and optionally filters by name.
func ListProjects(name string) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Project.NewListProjectsParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Project.ListProjects(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, nil
}

// DescribeProject returns a single project by name.
func DescribeProject(name string) (any, error) {
	respAny, err := ListProjects(name)
	if err != nil {
		return nil, err
	}
	resp, ok := respAny.(*cs.ListProjectsResponse)
	if !ok || resp == nil || len(resp.Projects) == 0 {
		return nil, fmt.Errorf("project %s not found", name)
	}
	return resp.Projects[0], nil
}

// DeleteProject deletes a project by name and returns its ID.
func DeleteProject(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	pid, err := ResolveProject(name)
	if err != nil {
		return "", err
	}
	dp := client.Project.NewDeleteProjectParams(pid)
	if _, err := client.Project.DeleteProject(dp); err != nil {
		return "", fmt.Errorf("failed to delete project %s: %w", name, err)
	}
	log.Printf("Project %s deleted from CloudStack (id=%s)", name, pid)
	return pid, nil
}

// ApplyProject creates a new project. Updates are intentionally unsupported.
func ApplyProject(name, displayText string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("project metadata.name is required")
	}
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	if existing, _, _ := client.Project.GetProjectByName(name); existing != nil {
		return "", fmt.Errorf("project %s already exists in CloudStack (id=%s); updates are not supported", name, existing.Id)
	}
	if displayText == "" {
		displayText = name
	}
	cp := client.Project.NewCreateProjectParams(displayText, name)
	resp, err := client.Project.CreateProject(cp)
	if err != nil {
		return "", fmt.Errorf("failed to create project %s: %w", name, err)
	}
	log.Printf("Created Project %s (id=%s)", name, resp.Id)
	return resp.Id, nil
}
