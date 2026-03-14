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

// ListVMs queries CloudStack and prints a table of VMs.
func ListVMs(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tTEMPLATE\tSERVICE OFFERING\tSTATUS")
	for _, v := range resp.VirtualMachines {
		id := v.Id
		tmpl := v.Templatename
		so := v.Serviceofferingname
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Name, id, tmpl, so, v.State)
	}
	w.Flush()
	return nil
}

// DescribeVM prints JSON for a VM identified by name.
func DescribeVM(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	params.SetName(name)
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return fmt.Errorf("vm %s not found", name)
	}
	data, _ := json.MarshalIndent(resp.VirtualMachines[0], "", "  ")
	log.Println(string(data))
	return nil
}

// DeleteVM deletes a VM by name in CloudStack.
func DeleteVM(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	params.SetName(name)
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return fmt.Errorf("vm %s not found", name)
	}
	vid := resp.VirtualMachines[0].Id
	delp := client.VirtualMachine.NewDestroyVirtualMachineParams(vid)
	if _, err := client.VirtualMachine.DestroyVirtualMachine(delp); err != nil {
		return fmt.Errorf("failed to delete vm %s: %w", name, err)
	}
	log.Printf("VM %s deleted from CloudStack (id=%s)", name, vid)
	return nil
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
func ApplyVirtualMachine(vm *v1.VirtualMachine) error {
	return ApplyVirtualMachineManaged(vm, false)
}

// ApplyVirtualMachineManaged deploys a VM and optionally tags it as managed_by=cloudstackctl
func ApplyVirtualMachineManaged(vm *v1.VirtualMachine, managed bool) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	// Check for existing VM with the same name - we don't support updates
	listParams := client.VirtualMachine.NewListVirtualMachinesParams()
	listParams.SetName(vm.Metadata.Name)
	listResp, lerr := client.VirtualMachine.ListVirtualMachines(listParams)
	if lerr == nil && listResp != nil && len(listResp.VirtualMachines) > 0 {
		return fmt.Errorf("virtualmachine %s already exists in CloudStack (id=%s); updates are not supported", vm.Metadata.Name, listResp.VirtualMachines[0].Id)
	}

	// Resolve potential name references to IDs for template, service offering, and networks
	// Resolve template and service offering names to IDs; require resolution.
	templateID, terr := ResolveTemplate(vm.Spec.Template)
	if terr != nil {
		return fmt.Errorf("failed to resolve template %s: %w", vm.Spec.Template, terr)
	}
	serviceOfferingID, soerr := ResolveServiceOffering(vm.Spec.ServiceOffering)
	if soerr != nil {
		return fmt.Errorf("failed to resolve service offering %s: %w", vm.Spec.ServiceOffering, soerr)
	}

	// Resolve network names to IDs where provided
	resolvedNets := make([]string, 0, len(vm.Spec.NetworkIDs))
	for _, n := range vm.Spec.NetworkIDs {
		nid, nerr := ResolveNetwork(n)
		if nerr != nil {
			return fmt.Errorf("failed to resolve network %s: %w", n, nerr)
		}
		resolvedNets = append(resolvedNets, nid)
	}

	params := client.VirtualMachine.NewDeployVirtualMachineParams(serviceOfferingID, templateID, "")
	params.SetName(vm.Metadata.Name)
	// If a zone is provided in the spec, try to resolve the zone name to an ID.
	// If resolution fails, assume the provided value is already an ID and use it.
	if vm.Spec.Zone != "" {
		zid, zerr := ResolveZone(vm.Spec.Zone)
		if zerr != nil {
			return fmt.Errorf("failed to resolve zone %s: %w", vm.Spec.Zone, zerr)
		}
		params.SetZoneid(zid)
	}
	if vm.Spec.Project != "" {
		// Accept either a project UUID or a project name; try resolving name first.
		if pid, perr := ResolveProject(vm.Spec.Project); perr == nil {
			params.SetProjectid(pid)
		} else {
			params.SetProjectid(vm.Spec.Project)
		}
	}
	if len(vm.Spec.NetworkIDs) > 0 {
		params.SetNetworkids(resolvedNets)
	}
	if len(vm.Spec.SSHKeys) > 0 {
		params.SetKeypairs(vm.Spec.SSHKeys)
	}
	// If parameters are provided, pass them as the CloudStack 'details' map
	// first (supported by the SDK).
	if vm.Spec.Parameters != nil {
		params.SetDetails(vm.Spec.Parameters)
	}

	resp, err := client.VirtualMachine.DeployVirtualMachine(params)
	if err != nil {
		return fmt.Errorf("cloudstack deploy error: %w", err)
	}
	log.Printf("Deployed VM %s (id=%s)", vm.Metadata.Name, resp.Id)

	if managed {
		tags := map[string]string{"managed_by": "cloudstackctl"}
		params := client.Resourcetags.NewCreateTagsParams([]string{resp.Id}, "UserVm", tags)
		if _, err := client.Resourcetags.CreateTags(params); err != nil {
			log.Printf("Warning: failed to create tag for VM %s (id=%s): %v", vm.Metadata.Name, resp.Id, err)
		}
	}

	return nil
}
