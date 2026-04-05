package handlers

import (
	"fmt"
	"log"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// ListSSHKeys lists SSH key pairs and returns the SDK response for callers to format.
func ListSSHKeys(name, project string, allProjects bool) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
	if err := setProjectOnParams(params, project); err != nil {
		return nil, err
	}
	setListAllOnParams(params, allProjects)
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeSSHKey returns the SSH keypair object from CloudStack by name.
func DescribeSSHKey(name, project string, allProjects bool) (any, error) {
	respAny, err := ListSSHKeys(name, project, allProjects)
	if err != nil {
		return nil, err
	}
	resp, _ := respAny.(*cs.ListSSHKeyPairsResponse)
	if resp == nil || len(resp.SSHKeyPairs) == 0 {
		return nil, fmt.Errorf("ssh key %s not found", name)
	}
	return resp.SSHKeyPairs[0], nil
}

// DeleteSSHKey deletes an SSH key by name.
func DeleteSSHKey(name, project string) (string, error) {
	respAny, err := ListSSHKeys(name, project, false)
	if err != nil {
		return "", err
	}
	resp, _ := respAny.(*cs.ListSSHKeyPairsResponse)
	if resp == nil || len(resp.SSHKeyPairs) == 0 {
		return "", fmt.Errorf("ssh key %s not found", name)
	}
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	dp := client.SSH.NewDeleteSSHKeyPairParams(name)
	if err := setProjectOnParams(dp, project); err != nil {
		return "", err
	}
	if _, err := client.SSH.DeleteSSHKeyPair(dp); err != nil {
		return "", fmt.Errorf("failed to delete ssh key %s: %w", name, err)
	}
	log.Printf("SSH key %s deleted from CloudStack", name)
	return name, nil
}

// ApplySSHKey ensures an SSHKey exists in CloudStack. Currently only discovery
// is supported; creating or registering public keys in controller mode is
// not implemented (use CLI standalone to register keys).
func ApplySSHKey(key *v1.SSHKey) (string, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	if key.Metadata.Name == "" {
		return "", fmt.Errorf("sshkey metadata.name is required")
	}

	// Check if key already exists by name
	listParams := client.SSH.NewListSSHKeyPairsParams()
	listParams.SetName(key.Metadata.Name)
	if err := setProjectOnParams(listParams, key.Metadata.Project); err != nil {
		return "", err
	}
	resp, err := client.SSH.ListSSHKeyPairs(listParams)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp != nil && len(resp.SSHKeyPairs) > 0 {
		return "", fmt.Errorf("sshkey %s already exists in CloudStack (fingerprint=%s); updates are not supported", key.Metadata.Name, resp.SSHKeyPairs[0].Fingerprint)
	}

	// Register the provided public key
	if key.Spec.PublicKey == "" {
		return "", fmt.Errorf("sshkey spec.publicKey is required to register a new key")
	}

	regParams := client.SSH.NewRegisterSSHKeyPairParams(key.Metadata.Name, key.Spec.PublicKey)
	if err := setProjectOnParams(regParams, key.Metadata.Project); err != nil {
		return "", err
	}
	if _, err := client.SSH.RegisterSSHKeyPair(regParams); err != nil {
		return "", fmt.Errorf("failed to register ssh key %s: %w", key.Metadata.Name, err)
	}
	log.Printf("Registered SSHKey %s", key.Metadata.Name)
	return key.Metadata.Name, nil
}

// ResolveSSHKey returns the SSH keypair name if present in CloudStack.
func ResolveSSHKey(name string) (string, error) {
	// If the value looks like a UUID, treat it as an ID and return it.
	if IsUUID(name) {
		return name, nil
	}

	client, err := cloudstack.NewClient()
	if err != nil {
		return "", fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return "", fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.SSHKeyPairs) == 0 {
		return "", fmt.Errorf("ssh key %s not found", name)
	}
	for _, k := range resp.SSHKeyPairs {
		if k.Name == name {
			return k.Name, nil
		}
	}
	return "", fmt.Errorf("ssh key %s not found", name)
}
