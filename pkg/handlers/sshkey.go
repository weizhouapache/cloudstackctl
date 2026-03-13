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

// ListSSHKeys prints SSH key pairs.
func ListSSHKeys() error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tFINGERPRINT")
	for _, k := range resp.SSHKeyPairs {
		fmt.Fprintf(w, "%s\t%s\n", k.Name, k.Fingerprint)
	}
	w.Flush()
	return nil
}

// DescribeSSHKey prints JSON for an SSH key by name.
func DescribeSSHKey(name string) error {
	client, err := cloudstack.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create CloudStack client: %w", err)
	}
	params := client.SSH.NewListSSHKeyPairsParams()
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
	params := client.SSH.NewListSSHKeyPairsParams()
	resp, err := client.SSH.ListSSHKeyPairs(params)
	if err != nil {
		return fmt.Errorf("cloudstack API error: %w", err)
	}
	if resp != nil {
		for _, k := range resp.SSHKeyPairs {
			if k.Name == key.Metadata.Name {
				return fmt.Errorf("sshkey %s already exists in CloudStack; updates are not supported", key.Metadata.Name)
			}
		}
	}
	return fmt.Errorf("creating or registering SSHKey from controller is not implemented; use CLI standalone to register a keypair")
}

// ResolveSSHKey returns the SSH keypair name if present in CloudStack.
func ResolveSSHKey(name string) (string, error) {
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
