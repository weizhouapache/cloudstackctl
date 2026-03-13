package handlers

import (
	"encoding/json"
	"fmt"
	"log"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"
)

// ApplyAffinityGroup ensures the AffinityGroup exists in CloudStack and creates
// it when missing. It uses the AffinityGroup spec.type to create the group.
func ApplyAffinityGroup(ag *v1.AffinityGroup) error {
	name := ag.Metadata.Name
	if name == "" {
		return fmt.Errorf("affinitygroup metadata.name is required")
	}
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	// Try to find by name
	existing, _, err := client.AffinityGroup.GetAffinityGroupByName(name)
	if err == nil && existing != nil {
		return fmt.Errorf("affinitygroup %s already exists in CloudStack (id=%s); updates are not supported", name, existing.Id)
	}

	// Create
	if ag.Spec.Type == "" {
		return fmt.Errorf("affinitygroup spec.type is required for creation")
	}
	p := client.AffinityGroup.NewCreateAffinityGroupParams(name, ag.Spec.Type)
	if _, err := client.AffinityGroup.CreateAffinityGroup(p); err != nil {
		return fmt.Errorf("failed to create affinity group: %w", err)
	}
	log.Printf("Created AffinityGroup %s", name)
	return nil
}

// ListAffinityGroups lists affinity groups in CloudStack.
func ListAffinityGroups() error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.AffinityGroup.NewListAffinityGroupsParams()
	resp, err := client.AffinityGroup.ListAffinityGroups(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	// Print header then rows
	fmt.Println("NAME\tID")
	for _, a := range resp.AffinityGroups {
		fmt.Printf("%s\t%s\n", a.Name, a.Id)
	}
	return nil
}

// DescribeAffinityGroup prints details for an affinity group by name.
func DescribeAffinityGroup(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.AffinityGroup.NewListAffinityGroupsParams()
	params.SetName(name)
	resp, err := client.AffinityGroup.ListAffinityGroups(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.AffinityGroups) == 0 {
		return fmt.Errorf("affinity group %s not found", name)
	}
	b, _ := json.MarshalIndent(resp.AffinityGroups[0], "", "  ")
	fmt.Println(string(b))
	return nil
}
