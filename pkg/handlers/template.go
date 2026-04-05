package handlers

import (
	"fmt"
	"log"

	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ListTemplates lists templates and returns the SDK response for callers to format.
func ListTemplates(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Template.NewListTemplatesParams("")
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Template.ListTemplates(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeTemplate returns the template object from CloudStack by name.
func DescribeTemplate(name, project string, allProjects bool) (any, error) {
	respAny, err := ListTemplates(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListTemplatesResponse)
	if resp == nil || len(resp.Templates) == 0 {
		return nil, fmt.Errorf("template %s not found", name)
	}
	return resp.Templates[0], nil
}

// DeleteTemplate deletes a template by name.
func DeleteTemplate(name, project string) (string, error) {
	respAny, err := ListTemplates(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListTemplatesResponse)
	if resp == nil || len(resp.Templates) == 0 {
		return "", fmt.Errorf("template %s not found", name)
	}
	tid := resp.Templates[0].Id
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	dp := client.Template.NewDeleteTemplateParams(tid)
	if _, err := client.Template.DeleteTemplate(dp); err != nil {
		return "", fmt.Errorf("failed to delete template %s: %w", name, err)
	}
	log.Printf("Template %s deleted from CloudStack (id=%s)", name, tid)
	return tid, nil
}

// ResolveTemplate returns the CloudStack template ID for a given template name.
func ResolveTemplate(name string) (string, error) {
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Template.NewListTemplatesParams("")
	params.SetName(name)
	params.SetTemplatefilter("all") // Search all templates (not just "featured" or "self") to allow resolving by name regardless of ownership or featured status.
	resp, err := client.Template.ListTemplates(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Templates) == 0 {
		return "", fmt.Errorf("template %s not found", name)
	}
	return resp.Templates[0].Id, nil
}
