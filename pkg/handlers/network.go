package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
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
func ListNetworks(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Network.NewListNetworksParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Network.ListNetworks(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeNetwork prints JSON for a single network identified by name.
func DescribeNetwork(name, project string, allProjects bool) (any, error) {
	respAny, err := ListNetworks(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListNetworksResponse)
	if resp == nil || len(resp.Networks) == 0 {
		return nil, fmt.Errorf("network %s not found", name)
	}
	return resp.Networks[0], nil
}

// DeleteNetwork deletes a network by name via CloudStack API.
func DeleteNetwork(name, project string) (string, error) {
	respAny, err := ListNetworks(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListNetworksResponse)
	if resp == nil || len(resp.Networks) == 0 {
		return "", fmt.Errorf("network %s not found", name)
	}
	nid := resp.Networks[0].Id
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	delp := client.Network.NewDeleteNetworkParams(nid)
	if _, err := client.Network.DeleteNetwork(delp); err != nil {
		return "", fmt.Errorf("failed to delete network %s: %w", name, err)
	}
	log.Printf("Network %s deleted from CloudStack (id=%s)", name, nid)
	return nid, nil
}

// ApplyNetwork applies or updates a Network resource using the CloudStack API.
// It searches for an existing network by name; if none is found, it creates one.
// If an existing network is found, it updates the description when it differs
// from the desired spec.
func ApplyNetwork(netRes *v1.Network) (string, error) {
	name := netRes.Metadata.Name
	if name == "" {
		return "", fmt.Errorf("network metadata.name is required")
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	// Try to find existing network by name
	listParams := client.Network.NewListNetworksParams()
	listParams.SetName(name)
	listParams.SetListall(true)
	if err := setProjectOnParams(listParams, netRes.Metadata.Project); err != nil {
		return "", err
	}
	listResp, err := client.Network.ListNetworks(listParams)
	if err != nil {
		return "", fmt.Errorf("failed to list networks: %w", err)
	}

	// Not found -> create
	if listResp == nil || len(listResp.Networks) == 0 {
		if netRes.Spec.NetworkOffering == "" || netRes.Spec.Zone == "" {
			return "", fmt.Errorf("network create requires spec.networkOffering and spec.zone in standalone mode")
		}
		// Resolve zone name to ID; require resolution or return an error.
		zoneID, zerr := ResolveZone(netRes.Spec.Zone)
		if zerr != nil {
			return "", fmt.Errorf("failed to resolve zone %s: %w", netRes.Spec.Zone, zerr)
		}
		// Resolve network offering name to ID; require resolution.
		offeringID, offErr := ResolveNetworkOffering(netRes.Spec.NetworkOffering)
		if offErr != nil {
			return "", fmt.Errorf("failed to resolve network offering %s: %w", netRes.Spec.NetworkOffering, offErr)
		}
		createParams := client.Network.NewCreateNetworkParams(name, offeringID, zoneID)
		if err := setProjectOnParams(createParams, netRes.Metadata.Project); err != nil {
			return "", err
		}
		if netRes.Spec.Description != "" {
			createParams.SetDisplaytext(netRes.Spec.Description)
		}

		// Pass bypassvlanoverlapcheck through to CloudStack (supported by API)
		createParams.SetBypassvlanoverlapcheck(netRes.Spec.BypassVlanOverlapCheck)

		// If shared network fields are present, set them on the create params.
		if netRes.Spec.Gateway != "" {
			createParams.SetGateway(netRes.Spec.Gateway)
		}
		if netRes.Spec.Netmask != "" {
			createParams.SetNetmask(netRes.Spec.Netmask)
		}
		if netRes.Spec.StartIP != "" {
			createParams.SetStartip(netRes.Spec.StartIP)
		}
		if netRes.Spec.EndIP != "" {
			createParams.SetEndip(netRes.Spec.EndIP)
		}
		if netRes.Spec.Vlan != nil {
			var vlanVal string
			switch v := netRes.Spec.Vlan.(type) {
			case string:
				vlanVal = v
			case int:
				vlanVal = strconv.Itoa(v)
			case int64:
				vlanVal = strconv.FormatInt(v, 10)
			case float64:
				// YAML numbers may be decoded as float64
				vlanVal = strconv.FormatInt(int64(v), 10)
			default:
				vlanVal = fmt.Sprintf("%v", v)
			}
			// Accept numeric VLANs like "1000" or the full URI "vlan://1000".
			// If the value already contains a scheme ("://"), trust it
			// (supports vlan://, vxlan://, etc.). Only numeric values
			// without a scheme get prefixed with "vlan://".
			if vlanVal != "" {
				if !strings.Contains(vlanVal, "://") {
					if _, err := strconv.Atoi(vlanVal); err == nil {
						vlanVal = "vlan://" + vlanVal
					}
				}
				createParams.SetVlan(vlanVal)
			}
		}
		resp, err := client.Network.CreateNetwork(createParams)
		if err != nil {
			return "", fmt.Errorf("cloudstack create network error: %w", err)
		}
		log.Printf("Created Network %s (id=%s)", name, resp.Id)
		return resp.Id, nil
	}
	// Resource exists — updates are not supported at this time
	existing := listResp.Networks[0]
	return "", fmt.Errorf("network %s already exists in CloudStack (id=%s); updates are not supported", name, existing.Id)
}
