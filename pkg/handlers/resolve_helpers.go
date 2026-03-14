package handlers

import (
	"fmt"

	"cloudstackctl/pkg/cloudstack"
)

// ResolveZone returns the CloudStack zone ID for a zone name.
func ResolveZone(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Zone.NewListZonesParams()
	params.SetName(name)
	resp, err := client.Zone.ListZones(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Zones) == 0 {
		return "", fmt.Errorf("zone %s not found", name)
	}
	return resp.Zones[0].Id, nil
}

// ResolveServiceOffering returns the service offering ID for a name.
func ResolveServiceOffering(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.ServiceOffering.NewListServiceOfferingsParams()
	params.SetName(name)
	resp, err := client.ServiceOffering.ListServiceOfferings(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.ServiceOfferings) == 0 {
		return "", fmt.Errorf("service offering %s not found", name)
	}
	return resp.ServiceOfferings[0].Id, nil
}

// ResolveDiskOffering returns the disk offering ID for a name.
func ResolveDiskOffering(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.DiskOffering.NewListDiskOfferingsParams()
	params.SetName(name)
	resp, err := client.DiskOffering.ListDiskOfferings(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.DiskOfferings) == 0 {
		return "", fmt.Errorf("disk offering %s not found", name)
	}
	return resp.DiskOfferings[0].Id, nil
}

// ResolveProject returns the CloudStack project ID for a given project name.
func ResolveProject(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Project.NewListProjectsParams()
	params.SetName(name)
	resp, err := client.Project.ListProjects(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Projects) == 0 {
		return "", fmt.Errorf("project %s not found", name)
	}
	return resp.Projects[0].Id, nil
}
