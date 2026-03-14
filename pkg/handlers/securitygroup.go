package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"
)

// ListSecurityGroups prints a table of security groups.
func ListSecurityGroups(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SecurityGroup.NewListSecurityGroupsParams()
	if name != "" {
		// Use SDK parameter to filter by security group name when supported
		params.SetSecuritygroupname(name)
	}
	resp, err := client.SecurityGroup.ListSecurityGroups(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tDESCRIPTION")
	for _, sg := range resp.SecurityGroups {
		fmt.Fprintf(w, "%s\t%s\t%s\n", sg.Name, sg.Id, sg.Description)
	}
	w.Flush()
	return nil
}

// DescribeSecurityGroup prints JSON for a security group by name.
func DescribeSecurityGroup(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	sg, _, err := client.SecurityGroup.GetSecurityGroupByName(name)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if sg == nil {
		return fmt.Errorf("security group %s not found", name)
	}
	data, _ := json.MarshalIndent(sg, "", "  ")
	log.Println(string(data))
	return nil
}

// DeleteSecurityGroup deletes a security group by name.
func DeleteSecurityGroup(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	sg, _, err := client.SecurityGroup.GetSecurityGroupByName(name)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if sg == nil {
		return fmt.Errorf("security group %s not found", name)
	}
	dp := client.SecurityGroup.NewDeleteSecurityGroupParams()
	dp.SetId(sg.Id)
	if _, err := client.SecurityGroup.DeleteSecurityGroup(dp); err != nil {
		return fmt.Errorf("failed to delete security group %s: %w", name, err)
	}
	log.Printf("Security group %s deleted from CloudStack (id=%s)", name, sg.Id)
	return nil
}

// ApplySecurityGroup ensures a security group exists in CloudStack. If the
// security group exists this is a no-op. Creating full security groups from
// controller currently requires more spec detail; return an informative error.
func ApplySecurityGroup(sg *v1.SecurityGroup) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	existing, _, err := client.SecurityGroup.GetSecurityGroupByName(sg.Metadata.Name)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("securitygroup %s already exists in CloudStack (id=%s); updates are not supported", sg.Metadata.Name, existing.Id)
	}
	// Create security group with optional description from metadata.annotations["description"]
	p := client.SecurityGroup.NewCreateSecurityGroupParams(sg.Metadata.Name)
	if sg.Metadata.Annotations != nil {
		if d, ok := sg.Metadata.Annotations["description"]; ok && d != "" {
			p.SetDescription(d)
		}
	}
	if _, err := client.SecurityGroup.CreateSecurityGroup(p); err != nil {
		return fmt.Errorf("failed to create security group: %w", err)
	}
	log.Printf("Created SecurityGroup %s", sg.Metadata.Name)
	return nil
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
