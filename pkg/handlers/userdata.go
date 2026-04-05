package handlers

import (
	"encoding/base64"
	"fmt"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ApplyUserData applies userdata script to a VM by name (uses VM reset userdata API).
func ApplyUserData(ud *v1.UserData) (string, error) {
	if ud.Metadata.Name == "" {
		return "", fmt.Errorf("userdata metadata.name is required (must be target VM name)")
	}
	if ud.Spec.Script == "" {
		return "", fmt.Errorf("userdata spec.script is required")
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	// Register UserData in CloudStack as a standalone UserData entry.
	// Use the SDK register/create method; CloudStack expects the content
	// to be provided as a string.
	// CloudStack expects userdata to be base64-encoded; encode before sending.
	encoded := base64.StdEncoding.EncodeToString([]byte(ud.Spec.Script))
	regParams := client.User.NewRegisterUserDataParams(ud.Metadata.Name, encoded)
	if err := setProjectOnParams(regParams, ud.Metadata.Project); err != nil {
		return "", err
	}
	resp, err := client.User.RegisterUserData(regParams)
	if err != nil {
		return "", fmt.Errorf("failed to register userdata %s: %w", ud.Metadata.Name, err)
	}
	if resp != nil {
		fmt.Printf("UserData %s registered (id=%s)\n", ud.Metadata.Name, resp.Id)
		return resp.Id, nil
	}
	fmt.Printf("UserData %s registered\n", ud.Metadata.Name)
	return "", nil
}

// ListUserData lists registered CloudStack UserData entries.
func ListUserData(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}

	params := client.User.NewListUserDataParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.User.ListUserData(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeUserData returns the UserData object from CloudStack by name.
func DescribeUserData(name, project string, allProjects bool) (any, error) {
	respAny, err := ListUserData(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListUserDataResponse)
	if resp == nil || len(resp.UserData) == 0 {
		return nil, fmt.Errorf("userdata %s not found", name)
	}
	return resp.UserData[0], nil
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
func DeleteUserData(name, project string) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	respAny, err := ListUserData(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListUserDataResponse)
	if resp == nil || len(resp.UserData) == 0 {
		return "", fmt.Errorf("userdata %s not found", name)
	}
	id := resp.UserData[0].Id
	dp := client.User.NewDeleteUserDataParams(id)
	if err := setProjectOnParams(dp, project); err != nil {
		return "", err
	}
	if _, err := client.User.DeleteUserData(dp); err != nil {
		return "", fmt.Errorf("failed to delete userdata %s: %w", name, err)
	}
	fmt.Printf("UserData %s deleted from CloudStack (id=%s)\n", name, id)
	return id, nil
}
