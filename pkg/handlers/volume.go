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

// ListVolumes prints a table of volumes.
func ListVolumes(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Volume.NewListVolumesParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Volume.ListVolumes(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tVM\tTYPE\tSTATUS")
	for _, v := range resp.Volumes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Name, v.Id, v.Vmname, v.Type, v.State)
	}
	w.Flush()
	return nil
}

// DescribeVolume prints JSON for a volume by name.
func DescribeVolume(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Volume.NewListVolumesParams()
	params.SetName(name)
	resp, err := client.Volume.ListVolumes(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Volumes) == 0 {
		return fmt.Errorf("volume %s not found", name)
	}
	data, _ := json.MarshalIndent(resp.Volumes[0], "", "  ")
	log.Println(string(data))
	return nil
}

// DeleteVolume deletes a volume by name.
func DeleteVolume(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Volume.NewListVolumesParams()
	params.SetName(name)
	resp, err := client.Volume.ListVolumes(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Volumes) == 0 {
		return fmt.Errorf("volume %s not found", name)
	}
	vid := resp.Volumes[0].Id
	dp := client.Volume.NewDeleteVolumeParams(vid)
	if _, err := client.Volume.DeleteVolume(dp); err != nil {
		return fmt.Errorf("failed to delete volume %s: %w", name, err)
	}
	log.Printf("Volume %s deleted from CloudStack (id=%s)", name, vid)
	return nil
}

// ApplyVolume attempts to ensure a Volume exists in CloudStack. For now
// the controller supports discovery (no-op if present) but does not perform
// full standalone creation of volumes (creation requires more spec fields).
func ApplyVolume(vol *v1.Volume) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	// Try discovery first
	params := client.Volume.NewListVolumesParams()
	params.SetName(vol.Metadata.Name)
	resp, err := client.Volume.ListVolumes(params)
	if err == nil && resp != nil && len(resp.Volumes) > 0 {
		return fmt.Errorf("volume %s already exists in CloudStack (id=%s); updates are not supported", vol.Metadata.Name, resp.Volumes[0].Id)
	}

	// If spec has disk offering and size, attempt creation
	if vol.Spec.DiskOffering != "" {
		diskOfferingID := vol.Spec.DiskOffering
		if doID, derr := ResolveDiskOffering(vol.Spec.DiskOffering); derr == nil {
			diskOfferingID = doID
		}

		cp := client.Volume.NewCreateVolumeParams()
		cp.SetName(vol.Metadata.Name)
		cp.SetDiskofferingid(diskOfferingID)
		if vol.Spec.SizeGB > 0 {
			cp.SetSize(int64(vol.Spec.SizeGB))
		}
		// If a zone is provided, resolve its name to an ID; fail if not resolvable.
		if vol.Spec.Zone != "" {
			zid, zerr := ResolveZone(vol.Spec.Zone)
			if zerr != nil {
				return fmt.Errorf("failed to resolve zone %s: %w", vol.Spec.Zone, zerr)
			}
			cp.SetZoneid(zid)
		}
		if _, err := client.Volume.CreateVolume(cp); err != nil {
			return fmt.Errorf("failed to create volume: %w", err)
		}
		log.Printf("Requested creation of Volume %s", vol.Metadata.Name)
		return nil
	}

	return fmt.Errorf("creating Volume from controller requires spec.diskOffering and spec.size; use CLI standalone if you need immediate creation")
}

// ResolveVolume returns the CloudStack volume ID for a given volume name.
func ResolveVolume(name string) (string, error) {
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Volume.NewListVolumesParams()
	params.SetName(name)
	resp, err := client.Volume.ListVolumes(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.Volumes) == 0 {
		return "", fmt.Errorf("volume %s not found", name)
	}
	return resp.Volumes[0].Id, nil
}
