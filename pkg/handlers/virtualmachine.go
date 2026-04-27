package handlers

import (
	"fmt"
	"log"
	"strings"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

func vmAttachedToAnyNetwork(vm *cs.VirtualMachine, networkIDs []string) bool {
	if vm == nil || len(networkIDs) == 0 {
		return false
	}
	requested := make(map[string]struct{}, len(networkIDs))
	for _, id := range networkIDs {
		if id != "" {
			requested[id] = struct{}{}
		}
	}
	for _, nic := range vm.Nic {
		if _, ok := requested[nic.Networkid]; ok {
			return true
		}
	}
	return false
}

func findExistingVMInScope(client *cs.CloudStackClient, name, project string, networkIDs []string) (*cs.VirtualMachine, error) {
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	params.SetName(name)
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return nil, fmt.Errorf("failed to list virtual machines: %w", err)
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return nil, nil
	}
	if len(networkIDs) == 0 {
		return resp.VirtualMachines[0], nil
	}
	for _, existing := range resp.VirtualMachines {
		if vmAttachedToAnyNetwork(existing, networkIDs) {
			return existing, nil
		}
	}
	return nil, nil
}

// ListVMs queries CloudStack and returns the SDK response for callers to format.
func ListVMs(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeVM prints JSON for a VM identified by name.

func DescribeVM(name, project string, allProjects bool) (any, error) {
	respAny, err := ListVMs(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListVirtualMachinesResponse)
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return nil, fmt.Errorf("vm %s not found", name)
	}
	return resp.VirtualMachines[0], nil
}

// DeleteVM deletes a VM by name in CloudStack.

func DeleteVM(name, project string) (string, error) {
	respAny, err := ListVMs(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListVirtualMachinesResponse)
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return "", fmt.Errorf("vm %s not found", name)
	}
	vid := resp.VirtualMachines[0].Id
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	delp := client.VirtualMachine.NewDestroyVirtualMachineParams(vid)
	delp.SetExpunge(true) // Permanently remove the VM instead of just stopping it
	if _, err := client.VirtualMachine.DestroyVirtualMachine(delp); err != nil {
		return "", fmt.Errorf("failed to delete vm %s: %w", name, err)
	}
	log.Printf("VM %s deleted from CloudStack (id=%s)", name, vid)
	return vid, nil
}

// ResolveVirtualMachine returns the CloudStack VM ID for a given VM name.
func ResolveVirtualMachine(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	params.SetName(name)
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return "", fmt.Errorf("vm %s not found", name)
	}
	return resp.VirtualMachines[0].Id, nil
}

// ApplyVirtualMachine deploys a virtual machine directly via CloudStack API.
// This is the standalone-path implementation used when the VM does not
// reference a managed VM spec.
func ApplyVirtualMachine(vm *v1.VirtualMachine) (string, error) {
	return ApplyVirtualMachineManaged(vm, false)
}

// ApplyVirtualMachineManaged deploys a VM and optionally tags it as managed_by=cloudstackctl
// Returns the CloudStack VM ID when successful.
func ApplyVirtualMachineManaged(vm *v1.VirtualMachine, managed bool) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	project := vm.Spec.Project
	if project == "" {
		project = vm.Metadata.Project
	}

	// Resolve potential name references to IDs for template, service offering, and networks
	// Resolve template and service offering names to IDs; require resolution.
	templateID, terr := ResolveTemplate(vm.Spec.Template)
	if terr != nil {
		return "", fmt.Errorf("failed to resolve template %s: %w", vm.Spec.Template, terr)
	}
	serviceOfferingID, soerr := ResolveServiceOffering(vm.Spec.ServiceOffering)
	if soerr != nil {
		return "", fmt.Errorf("failed to resolve service offering %s: %w", vm.Spec.ServiceOffering, soerr)
	}

	// Resolve network names to IDs where provided
	resolvedNets := make([]string, 0, len(vm.Spec.Networks))
	for _, n := range vm.Spec.Networks {
		nid, nerr := ResolveNetwork(n)
		if nerr != nil {
			return "", fmt.Errorf("failed to resolve network %s: %w", n, nerr)
		}
		resolvedNets = append(resolvedNets, nid)
	}

	// For create via apply, only treat the VM as existing when it is found in the
	// same project scope and, when requested, attached to one of the requested networks.
	existingVM, lerr := findExistingVMInScope(client, vm.Metadata.Name, project, resolvedNets)
	if lerr != nil {
		return "", lerr
	}
	if existingVM != nil {
		if project != "" {
			return "", fmt.Errorf("virtualmachine %s already exists in project %s (id=%s); updates are not supported", vm.Metadata.Name, project, existingVM.Id)
		}
		return "", fmt.Errorf("virtualmachine %s already exists in CloudStack (id=%s); updates are not supported", vm.Metadata.Name, existingVM.Id)
	}

	params := client.VirtualMachine.NewDeployVirtualMachineParams(serviceOfferingID, templateID, "")
	params.SetName(vm.Metadata.Name)
	// If a zone is provided in the spec, try to resolve the zone name to an ID.
	// If resolution fails, assume the provided value is already an ID and use it.
	if vm.Spec.Zone != "" {
		zid, zerr := ResolveZone(vm.Spec.Zone)
		if zerr != nil {
			return "", fmt.Errorf("failed to resolve zone %s: %w", vm.Spec.Zone, zerr)
		}
		params.SetZoneid(zid)
	}
	if err := setProjectOnParams(params, project); err != nil {
		return "", err
	}
	if len(vm.Spec.Networks) > 0 {
		params.SetNetworkids(resolvedNets)
	}
	if len(vm.Spec.SSHKeys) > 0 {
		params.SetKeypairs(vm.Spec.SSHKeys)
	}
	// Apply security groups if provided: resolve names to IDs and set them.
	if len(vm.Spec.SecurityGroups) > 0 {
		sgIDs := []string{}
		for _, s := range vm.Spec.SecurityGroups {
			id, serr := ResolveSecurityGroup(s)
			if serr != nil {
				return "", fmt.Errorf("failed to resolve security group %s: %w", s, serr)
			}
			sgIDs = append(sgIDs, id)
		}
		params.SetSecuritygroupids(sgIDs)
	}
	// Resolve affinity group names to IDs and attach them to the deploy params.
	if len(vm.Spec.AffinityGroups) > 0 {
		agIDs := make([]string, 0, len(vm.Spec.AffinityGroups))
		for _, ag := range vm.Spec.AffinityGroups {
			id, agerr := ResolveAffinityGroup(ag)
			if agerr != nil {
				return "", fmt.Errorf("failed to resolve affinity group %s: %w", ag, agerr)
			}
			agIDs = append(agIDs, id)
		}
		params.SetAffinitygroupids(agIDs)
	}
	// Resolve userdata names to IDs; CloudStack accepts a comma-separated list.
	if len(vm.Spec.UserDataRefs) > 0 {
		udIDs := make([]string, 0, len(vm.Spec.UserDataRefs))
		for _, ud := range vm.Spec.UserDataRefs {
			id, uderr := ResolveUserData(ud)
			if uderr != nil {
				return "", fmt.Errorf("failed to resolve userdata %s: %w", ud, uderr)
			}
			udIDs = append(udIDs, id)
		}
		params.SetUserdataid(strings.Join(udIDs, ","))
	}
	// If parameters are provided, pass them as the CloudStack 'details' map
	// first (supported by the SDK).
	if vm.Spec.Parameters != nil {
		params.SetDetails(vm.Spec.Parameters)
	}

	// Prepare root volume or disk offering on deploy
	if len(vm.Spec.Volumes) > 0 {
		if err := PrepareVolumesForDeploy(client, params, vm.Spec.Volumes); err != nil {
			return "", fmt.Errorf("failed to prepare volumes for deploy: %w", err)
		}
	}

	resp, err := client.VirtualMachine.DeployVirtualMachine(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack deploy error: %w", err)
	}
	vid := resp.Id
	log.Printf("Deployed VM %s (id=%s)", vm.Metadata.Name, vid)

	if managed {
		tags := map[string]string{"managed_by": "cloudstackctl"}
		tparams := client.Resourcetags.NewCreateTagsParams([]string{vid}, "UserVm", tags)
		if _, terr := client.Resourcetags.CreateTags(tparams); terr != nil {
			log.Printf("Warning: failed to create tag for VM %s (id=%s): %v", vm.Metadata.Name, vid, terr)
		}
	}

	return vid, nil
}
