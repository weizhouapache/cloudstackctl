package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"
)

// ApplyUserData applies userdata script to a VM by name (uses VM reset userdata API).
func ApplyUserData(ud *v1.UserData) error {
	if ud.Metadata.Name == "" {
		return fmt.Errorf("userdata metadata.name is required (must be target VM name)")
	}
	if ud.Spec.Script == "" {
		return fmt.Errorf("userdata spec.script is required")
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	// Find VM by name
	params := client.VirtualMachine.NewListVirtualMachinesParams()
	params.SetName(ud.Metadata.Name)
	resp, err := client.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return fmt.Errorf("no VM found with name %s", ud.Metadata.Name)
	}
	vm := resp.VirtualMachines[0]

	// Reset user data for the found VM (CloudStack expects VM to be stopped).
	rp := client.VirtualMachine.NewResetUserDataForVirtualMachineParams(vm.Id)
	rp.SetUserdata(ud.Spec.Script)
	if _, err := client.VirtualMachine.ResetUserDataForVirtualMachine(rp); err != nil {
		return fmt.Errorf("failed to reset userdata for VM %s (id=%s): %w", ud.Metadata.Name, vm.Id, err)
	}

	fmt.Printf("UserData applied to VM %s (id=%s)\n", ud.Metadata.Name, vm.Id)
	return nil
}

// ListUserData lists registered CloudStack UserData entries.
func ListUserData(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	params := client.User.NewListUserDataParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.User.ListUserData(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}

	// Print header and rows using tabwriter for aligned columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tPROJECT\tACCOUNT")
	for _, u := range resp.UserData {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.Name, u.Id, u.Project, u.Account)
	}
	w.Flush()
	return nil
}

// DescribeUserData prints details for a UserData entry by name.
func DescribeUserData(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	params := client.User.NewListUserDataParams()
	params.SetName(name)
	resp, err := client.User.ListUserData(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.UserData) == 0 {
		return fmt.Errorf("userdata %s not found", name)
	}
	data, _ := json.MarshalIndent(resp.UserData[0], "", "  ")
	fmt.Println(string(data))
	return nil
}

// ResolveUserData returns the CloudStack UserData ID for a given name.
func ResolveUserData(name string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.User.NewListUserDataParams()
	params.SetName(name)
	resp, err := client.User.ListUserData(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.UserData) == 0 {
		return "", fmt.Errorf("userdata %s not found", name)
	}
	return resp.UserData[0].Id, nil
}
