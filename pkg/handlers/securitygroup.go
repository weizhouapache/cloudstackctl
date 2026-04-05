package handlers

import (
	"fmt"
	"log"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ListSecurityGroups lists security groups and returns the SDK response for callers to format.
func ListSecurityGroups(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SecurityGroup.NewListSecurityGroupsParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		// Use SDK parameter to filter by security group name when supported
		params.SetSecuritygroupname(name)
	}
	resp, err := client.SecurityGroup.ListSecurityGroups(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeSecurityGroup returns the security group object from CloudStack by name.
func DescribeSecurityGroup(name, project string, allProjects bool) (any, error) {
	respAny, err := ListSecurityGroups(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListSecurityGroupsResponse)
	if resp == nil || len(resp.SecurityGroups) == 0 {
		return nil, fmt.Errorf("security group %s not found", name)
	}
	return resp.SecurityGroups[0], nil
}

// DeleteSecurityGroup deletes a security group by name.
func DeleteSecurityGroup(name, project string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	respAny, err := ListSecurityGroups(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListSecurityGroupsResponse)
	if resp == nil || len(resp.SecurityGroups) == 0 {
		return "", fmt.Errorf("security group %s not found", name)
	}
	sg := resp.SecurityGroups[0]
	dp := client.SecurityGroup.NewDeleteSecurityGroupParams()
	dp.SetId(sg.Id)
	if err := setProjectOnParams(dp, project); err != nil {
		return "", err
	}
	if _, err := client.SecurityGroup.DeleteSecurityGroup(dp); err != nil {
		return "", fmt.Errorf("failed to delete security group %s: %w", name, err)
	}
	log.Printf("Security group %s deleted from CloudStack (id=%s)", name, sg.Id)
	return sg.Id, nil
}

// ApplySecurityGroup ensures a security group exists in CloudStack. If the
// security group exists this is a no-op. Creating full security groups from
// controller currently requires more spec detail; return an informative error.
func ApplySecurityGroup(sg *v1.SecurityGroup) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	existing, _, err := client.SecurityGroup.GetSecurityGroupByName(sg.Metadata.Name)
	if existing != nil {
		return "", fmt.Errorf("securitygroup %s already exists in CloudStack (id=%s); updates are not supported", sg.Metadata.Name, existing.Id)
	}
	// Create security group with optional description from metadata.annotations["description"]
	p := client.SecurityGroup.NewCreateSecurityGroupParams(sg.Metadata.Name)
	if err := setProjectOnParams(p, sg.Metadata.Project); err != nil {
		return "", err
	}
	if sg.Metadata.Annotations != nil {
		if d, ok := sg.Metadata.Annotations["description"]; ok && d != "" {
			p.SetDescription(d)
		}
	}
	resp, err := client.SecurityGroup.CreateSecurityGroup(p)
	if err != nil {
		return "", fmt.Errorf("failed to create security group: %w", err)
	}
	return resp.Id, nil
}

// ResolveSecurityGroup returns the CloudStack security group ID for a given name.
func ResolveSecurityGroup(name string) (string, error) {
	// UUID inputs are treated as IDs.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	sg, _, err := client.SecurityGroup.GetSecurityGroupByName(name)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if sg == nil {
		return "", fmt.Errorf("security group %s not found", name)
	}
	return sg.Id, nil
}
