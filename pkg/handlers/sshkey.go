package handlers

import (
	"encoding/json"
	"fmt"
	"log"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/cloudstack"
)

// ListSSHKeys lists SSH key pairs and returns the SDK response for callers to format.
func ListSSHKeys(name string) (any, error) {
	client, err := cloudstack.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return nil, fmt.Errorf("cloudstack API error: %w", err)
	}
	return resp, err
}

// DescribeSSHKey prints JSON for an SSH key by name.
func DescribeSSHKey(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.SSHKeyPairs) == 0 {
		return fmt.Errorf("ssh key %s not found", name)
	}
	data, _ := json.MarshalIndent(resp.SSHKeyPairs[0], "", "  ")
	log.Println(string(data))
	return nil
}

// DeleteSSHKey deletes an SSH key by name.
func DeleteSSHKey(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
	if name != "" {
		params.SetName(name)
	}
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp == nil || len(resp.SSHKeyPairs) == 0 {
		return fmt.Errorf("ssh key %s not found", name)
	}
	dp := client.SSH.NewDeleteSSHKeyPairParams(name)
	if _, err := client.SSH.DeleteSSHKeyPair(dp); err != nil {
		return fmt.Errorf("failed to delete ssh key %s: %w", name, err)
	}
	log.Printf("SSH key %s deleted from CloudStack", name)
	return nil
}

// ApplySSHKey ensures an SSHKey exists in CloudStack. Currently only discovery
// is supported; creating or registering public keys in controller mode is
// not implemented (use CLI standalone to register keys).
func ApplySSHKey(key *v1.SSHKey) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	if key.Metadata.Name == "" {
		return fmt.Errorf("sshkey metadata.name is required")
	}

	// Check if key already exists by name
	listParams := client.SSH.NewListSSHKeyPairsParams()
	listParams.SetName(key.Metadata.Name)
	resp, err := client.SSH.ListSSHKeyPairs(listParams)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp != nil && len(resp.SSHKeyPairs) > 0 {
		return fmt.Errorf("sshkey %s already exists in CloudStack (fingerprint=%s); updates are not supported", key.Metadata.Name, resp.SSHKeyPairs[0].Fingerprint)
	}

	// Register the provided public key
	if key.Spec.PublicKey == "" {
		return fmt.Errorf("sshkey spec.publicKey is required to register a new key")
	}

	regParams := client.SSH.NewRegisterSSHKeyPairParams(key.Metadata.Name, key.Spec.PublicKey)
	if _, err := client.SSH.RegisterSSHKeyPair(regParams); err != nil {
		return fmt.Errorf("failed to register ssh key %s: %w", key.Metadata.Name, err)
	}
	log.Printf("Registered SSHKey %s", key.Metadata.Name)
	return nil
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
