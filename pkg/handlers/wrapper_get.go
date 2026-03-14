package handlers

import (
	"encoding/json"
	"fmt"
)

// GetCloudStackResource lists or describes an unmanaged CloudStack resource
// based on a raw JSON payload. The payload should include `kind` and may
// include either a top-level `name` or `metadata.name` to request a single
// resource. If no name is provided, the list function is called with an
// empty name to list all resources of the given kind.
func GetCloudStackResource(raw []byte) error {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("invalid resource JSON: %w", err)
	}

	kind, _ := meta["kind"].(string)

	// Extract name from top-level `name` or `metadata.name` if present
	name := ""
	if n, ok := meta["name"].(string); ok {
		name = n
	}
	if m, ok := meta["metadata"].(map[string]interface{}); ok {
		if n, ok := m["name"].(string); ok {
			name = n
		}
	}

	switch kind {
	case "VirtualMachine":
		return ListVMs(name)
	case "Network":
		return ListNetworks(name)
	case "Template":
		return ListTemplates(name)
	case "Volume":
		return ListVolumes(name)
	case "SSHKey":
		return ListSSHKeys(name)
	case "SecurityGroup":
		return ListSecurityGroups(name)
	case "AffinityGroup":
		return ListAffinityGroups(name)
	case "UserData":
		return ListUserData(name)
	default:
		return fmt.Errorf("unsupported resource kind for cloudstack get: %s", kind)
	}
}
