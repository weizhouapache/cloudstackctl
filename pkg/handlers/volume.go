package handlers

import (
	"fmt"
	"log"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ListVolumes prints a table of volumes.
func ListVolumes(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.Volume.NewListVolumesParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.Volume.ListVolumes(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeVolume returns the volume object from CloudStack by name.
func DescribeVolume(name, project string, allProjects bool) (any, error) {
	respAny, err := ListVolumes(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListVolumesResponse)
	if resp == nil || len(resp.Volumes) == 0 {
		return nil, fmt.Errorf("volume %s not found", name)
	}
	return resp.Volumes[0], nil
}

// DeleteVolume deletes a volume by name and returns the deleted CloudStack ID.
func DeleteVolume(name, project string) (string, error) {
	respAny, err := ListVolumes(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListVolumesResponse)
	if resp == nil || len(resp.Volumes) == 0 {
		return "", fmt.Errorf("volume %s not found", name)
	}
	vid := resp.Volumes[0].Id
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	dp := client.Volume.NewDeleteVolumeParams(vid)
	if _, err := client.Volume.DeleteVolume(dp); err != nil {
		return "", fmt.Errorf("failed to delete volume %s: %w", name, err)
	}
	log.Printf("Volume %s deleted from CloudStack (id=%s)", name, vid)
	return vid, nil
}

// ApplyVolume attempts to ensure a Volume exists in CloudStack. For now
// the controller supports discovery (no-op if present) but does not perform
// full standalone creation of volumes (creation requires more spec fields).
// Returns the created Volume ID when applicable.
func ApplyVolume(vol *v1.Volume) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	// Try discovery first
	params := client.Volume.NewListVolumesParams()
	params.SetName(vol.Metadata.Name)
	if err := setProjectOnParams(params, vol.Metadata.Project); err != nil {
		return "", err
	}
	resp, err := client.Volume.ListVolumes(params)
	if err == nil && resp != nil && len(resp.Volumes) > 0 {
		return "", fmt.Errorf("volume %s already exists in CloudStack (id=%s); updates are not supported", vol.Metadata.Name, resp.Volumes[0].Id)
	}

	// If spec has disk offering and size, attempt creation
	if vol.Spec.DiskOffering != "" {
		diskOfferingID := vol.Spec.DiskOffering
		if doID, derr := ResolveDiskOffering(vol.Spec.DiskOffering); derr == nil {
			diskOfferingID = doID
		}

		cp := client.Volume.NewCreateVolumeParams()
		cp.SetName(vol.Metadata.Name)
		if err := setProjectOnParams(cp, vol.Metadata.Project); err != nil {
			return "", err
		}
		cp.SetDiskofferingid(diskOfferingID)
		if vol.Spec.SizeGB > 0 {
			cp.SetSize(int64(vol.Spec.SizeGB))
		}
		// If a zone is provided, resolve its name to an ID; fail if not resolvable.
		if vol.Spec.Zone != "" {
			zid, zerr := ResolveZone(vol.Spec.Zone)
			if zerr != nil {
				return "", fmt.Errorf("failed to resolve zone %s: %w", vol.Spec.Zone, zerr)
			}
			cp.SetZoneid(zid)
		}
		resp, err := client.Volume.CreateVolume(cp)
		if err != nil {
			return "", fmt.Errorf("failed to create volume: %w", err)
		}
		if resp != nil {
			msg := fmt.Sprintf("Volume \"%s\" (ID: %s) has been created", vol.Metadata.Name, resp.Id)
			log.Println(msg)
			return resp.Id, nil
		}
		log.Printf("Requested creation of Volume %s", vol.Metadata.Name)
		return "", nil
	}

	return "", fmt.Errorf("creating Volume from controller requires spec.diskOffering and spec.size; use CLI standalone if you need immediate creation")
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

// PrepareVolumesForDeploy inspects the provided volume specs and adjusts
// DeployVirtualMachineParams accordingly. It will create root volumes when
// necessary and set the appropriate deploy params (`volumeid` or
// `diskofferingid`). Data disks are translated into datadiskofferinglist and
// datadisksdetails so CloudStack will create them at deploy time.
func PrepareVolumesForDeploy(client *cs.CloudStackClient, params *cs.DeployVirtualMachineParams, vols []v1.VolumeSpec) error {
	if client == nil || params == nil {
		return nil
	}
	dataIdx := 1
	datadisksDetails := []map[string]string{}

	for _, vol := range vols {
		typ := vol.Type
		if typ == "" {
			typ = "data"
		}

		if typ == "root" {
			// If an existing volume ID provided, use it as the root volume
			if vol.ID != "" {
				vid := vol.ID
				if !IsUUID(vid) {
					if rid, err := ResolveVolume(vid); err == nil {
						vid = rid
					}
				}
				params.SetVolumeid(vid)
				// continue to allow data disks to be processed
				continue
			}

			// If size provided, ask deploy to set root disk size
			if vol.SizeGB > 0 {
				params.SetRootdisksize(int64(vol.SizeGB))
			}

			// If disk offering provided, prefer override root disk offering
			if vol.DiskOffering != "" {
				do := vol.DiskOffering
				if did, derr := ResolveDiskOffering(vol.DiskOffering); derr == nil {
					do = did
				}
				params.SetOverridediskofferingid(do)
			}

			continue
		}

		// Data disk handling: if ID provided, we will attach post-deploy; if a disk offering
		// is provided, instruct deploy to create data disk(s) via datadiskofferinglist
		if vol.ID != "" {
			// existing volume will be attached post-deploy by caller
			continue
		}
		if vol.DiskOffering != "" {
			do := vol.DiskOffering
			if did, derr := ResolveDiskOffering(vol.DiskOffering); derr == nil {
				do = did
			}
			diskDetail := map[string]string{
				"deviceid":       fmt.Sprintf("%d", dataIdx),
				"diskofferingid": do,
			}
			if vol.SizeGB > 0 {
				diskDetail["size"] = fmt.Sprintf("%d", vol.SizeGB)
			}
			datadisksDetails = append(datadisksDetails, diskDetail)
			dataIdx++
		}
	}

	if len(datadisksDetails) > 0 {
		params.SetDatadisksdetails(datadisksDetails)
	}

	return nil
}
