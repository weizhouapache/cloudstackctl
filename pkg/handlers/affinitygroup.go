package handlers

import (
	"fmt"
	"log"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ApplyAffinityGroup ensures the AffinityGroup exists in CloudStack and creates
// it when missing. It uses the AffinityGroup spec.type to create the group.
func ApplyAffinityGroup(ag *v1.AffinityGroup) (string, error) {
	name := ag.Metadata.Name
	if name == "" {
		return "", fmt.Errorf("affinitygroup metadata.name is required")
	}
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	// Try to find by name in the provided project scope.
	listParams := client.AffinityGroup.NewListAffinityGroupsParams()
	listParams.SetName(name)
	if err := setProjectOnParams(listParams, ag.Metadata.Project); err != nil {
		return "", err
	}
	listResp, err := client.AffinityGroup.ListAffinityGroups(listParams)
	if err != nil {
		return "", fmt.Errorf("failed to list affinity groups: %w", err)
	}
	if listResp != nil && len(listResp.AffinityGroups) > 0 {
		existing := listResp.AffinityGroups[0]
		if ag.Metadata.Project != "" {
			return "", fmt.Errorf("affinitygroup %s already exists in project %s (id=%s); updates are not supported", name, ag.Metadata.Project, existing.Id)
		}
		return "", fmt.Errorf("affinitygroup %s already exists in CloudStack (id=%s); updates are not supported", name, existing.Id)
	}

	// Create
	if ag.Spec.Type == "" {
		return "", fmt.Errorf("affinitygroup spec.type is required for creation")
	}
	p := client.AffinityGroup.NewCreateAffinityGroupParams(name, ag.Spec.Type)
	if err := setProjectOnParams(p, ag.Metadata.Project); err != nil {
		return "", err
	}
	resp, err := client.AffinityGroup.CreateAffinityGroup(p)
	if err != nil {
		return "", fmt.Errorf("failed to create affinity group: %w", err)
	}
	return resp.Id, nil
}

// ListAffinityGroups lists affinity groups in CloudStack.
func ListAffinityGroups(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.AffinityGroup.NewListAffinityGroupsParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.AffinityGroup.ListAffinityGroups(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeAffinityGroup returns the affinity group object from CloudStack by name.
func DescribeAffinityGroup(name, project string, allProjects bool) (any, error) {
	respAny, err := ListAffinityGroups(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListAffinityGroupsResponse)
	if resp == nil || len(resp.AffinityGroups) == 0 {
		return nil, fmt.Errorf("affinity group %s not found", name)
	}
	return resp.AffinityGroups[0], nil
}

// ResolveAffinityGroup returns the CloudStack affinity group ID for a given name.
func ResolveAffinityGroup(name string) (string, error) {
	// Treat UUID inputs as IDs.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	existing, _, err := client.AffinityGroup.GetAffinityGroupByName(name)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if existing == nil {
		return "", fmt.Errorf("affinity group %s not found", name)
	}
	return existing.Id, nil
}

// DeleteAffinityGroup deletes an affinity group by name.
func DeleteAffinityGroup(name, project string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	respAny, err := ListAffinityGroups(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListAffinityGroupsResponse)
	if resp == nil || len(resp.AffinityGroups) == 0 {
		return "", fmt.Errorf("affinity group %s not found", name)
	}
	existing := resp.AffinityGroups[0]
	dp := client.AffinityGroup.NewDeleteAffinityGroupParams()
	dp.SetId(existing.Id)
	if err := setProjectOnParams(dp, project); err != nil {
		return "", err
	}
	if _, err := client.AffinityGroup.DeleteAffinityGroup(dp); err != nil {
		return "", fmt.Errorf("failed to delete affinity group %s: %w", name, err)
	}
	log.Printf("AffinityGroup %s deleted from CloudStack (id=%s)", name, existing.Id)
	return existing.Id, nil
}
