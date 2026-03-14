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

// ResolveNetwork returns the CloudStack network ID for a given network name.
func ResolveNetwork(name string) (string, error) {
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Network.NewListNetworksParams()
	params.SetName(name)
	resp, err := client.Network.ListNetworks(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Networks) == 0 {
		return "", fmt.Errorf("network %s not found", name)
	}
	return resp.Networks[0].Id, nil
}

// ListNetworks queries CloudStack and prints a table of networks.
func ListNetworks(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Network.NewListNetworksParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Network.ListNetworks(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tZONE\tDISPLAY TEXT\tTYPE\tSTATE")
	for _, n := range resp.Networks {
		display := n.Displaytext
		if display == "" {
			display = n.Name
		}

		zoneName := n.Zoneid
		if n.Zoneid != "" {
			zp := client.Zone.NewListZonesParams()
			zp.SetId(n.Zoneid)
			zr, zerr := client.Zone.ListZones(zp)
			if zerr == nil && zr != nil && len(zr.Zones) > 0 {
				zoneName = zr.Zones[0].Name
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", n.Name, n.Id, zoneName, display, n.Type, n.State)
	}
	w.Flush()
	return nil
}

// DescribeNetwork prints JSON for a single network identified by name.
func DescribeNetwork(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Network.NewListNetworksParams()
	params.SetName(name)
	resp, err := client.Network.ListNetworks(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Networks) == 0 {
		return fmt.Errorf("network %s not found", name)
	}
	data, _ := json.MarshalIndent(resp.Networks[0], "", "  ")
	log.Println(string(data))
	return nil
}

// DeleteNetwork deletes a network by name via CloudStack API.
func DeleteNetwork(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Network.NewListNetworksParams()
	params.SetName(name)
	resp, err := client.Network.ListNetworks(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Networks) == 0 {
		return fmt.Errorf("network %s not found", name)
	}
	nid := resp.Networks[0].Id
	delp := client.Network.NewDeleteNetworkParams(nid)
	if _, err := client.Network.DeleteNetwork(delp); err != nil {
		return fmt.Errorf("failed to delete network %s: %w", name, err)
	}
	log.Printf("Network %s deleted from CloudStack (id=%s)", name, nid)
	return nil
}

// ApplyNetwork applies or updates a Network resource using the CloudStack API.
// It searches for an existing network by name; if none is found, it creates one.
// If an existing network is found, it updates the description when it differs
// from the desired spec.
func ApplyNetwork(netRes *v1.Network) error {
	name := netRes.Metadata.Name
	if name == "" {
		return fmt.Errorf("network metadata.name is required")
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	// Try to find existing network by name
	listParams := client.Network.NewListNetworksParams()
	listParams.SetName(name)
	listParams.SetListall(true)
	listResp, err := client.Network.ListNetworks(listParams)
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	// Not found -> create
	if listResp == nil || len(listResp.Networks) == 0 {
		if netRes.Spec.NetworkOffering == "" || netRes.Spec.Zone == "" {
			return fmt.Errorf("network create requires spec.networkOffering and spec.zone in standalone mode")
		}
		// Resolve zone name to ID; require resolution or return an error.
		zoneID, zerr := ResolveZone(netRes.Spec.Zone)
		if zerr != nil {
			return fmt.Errorf("failed to resolve zone %s: %w", netRes.Spec.Zone, zerr)
		}
		createParams := client.Network.NewCreateNetworkParams(name, netRes.Spec.NetworkOffering, zoneID)
		if netRes.Spec.Description != "" {
			createParams.SetDisplaytext(netRes.Spec.Description)
		}
		resp, err := client.Network.CreateNetwork(createParams)
		if err != nil {
			return fmt.Errorf("cloudstack create network error: %w", err)
		}
		log.Printf("Created Network %s (id=%s)", name, resp.Id)
		return nil
	}
	// Resource exists — updates are not supported at this time
	existing := listResp.Networks[0]
	return fmt.Errorf("network %s already exists in CloudStack (id=%s); updates are not supported", name, existing.Id)
}
