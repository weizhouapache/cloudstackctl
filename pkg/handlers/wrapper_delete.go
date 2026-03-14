package handlers

import (
	"encoding/json"
	"fmt"
)

// DeleteCloudStackResource deletes an unmanaged CloudStack resource based on
// a raw JSON payload containing `kind` and `name` (or metadata.name).
// This wrapper is for standalone CLI mode.
func DeleteCloudStackResource(raw []byte) error {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("invalid resource JSON: %w", err)
	}

	kind, _ := meta["kind"].(string)
	name := ""
	if n, ok := meta["name"].(string); ok {
		name = n
	}
	if m, ok := meta["metadata"].(map[string]interface{}); ok {
		if n, ok := m["name"].(string); ok {
			name = n
		}
	}

	if name == "" {
		return fmt.Errorf("missing name for delete operation")
	}

	switch kind {
	case "VirtualMachine":
		return DeleteVM(name)
	case "Network":
		return DeleteNetwork(name)
	case "Volume":
		return DeleteVolume(name)
	case "SSHKey":
		return DeleteSSHKey(name)
	case "SecurityGroup":
		return DeleteSecurityGroup(name)
	case "Template":
		return DeleteTemplate(name)
	case "Snapshot":
		return DeleteSnapshot(name)
	default:
		return fmt.Errorf("unsupported resource kind for standalone delete: %s", kind)
	}
}
