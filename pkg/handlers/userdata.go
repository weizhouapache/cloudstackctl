package handlers

import (
	"encoding/base64"
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
	// Register UserData in CloudStack as a standalone UserData entry.
	// Use the SDK register/create method; CloudStack expects the content
	// to be provided as a string.
	// CloudStack expects userdata to be base64-encoded; encode before sending.
	encoded := base64.StdEncoding.EncodeToString([]byte(ud.Spec.Script))
	regParams := client.User.NewRegisterUserDataParams(ud.Metadata.Name, encoded)
	resp, err := client.User.RegisterUserData(regParams)
	if err != nil {
		return fmt.Errorf("failed to register userdata %s: %w", ud.Metadata.Name, err)
	}
	if resp != nil {
		fmt.Printf("UserData %s registered (id=%s)\n", ud.Metadata.Name, resp.Id)
	} else {
		fmt.Printf("UserData %s registered\n", ud.Metadata.Name)
	}
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
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

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

// DeleteUserData deletes a UserData entry by name.
func DeleteUserData(name string) error {
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
	id := resp.UserData[0].Id
	dp := client.User.NewDeleteUserDataParams(id)
	if _, err := client.User.DeleteUserData(dp); err != nil {
		return fmt.Errorf("failed to delete userdata %s: %w", name, err)
	}
	fmt.Printf("UserData %s deleted from CloudStack (id=%s)\n", name, id)
	return nil
}
