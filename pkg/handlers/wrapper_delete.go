package handlers

import (
	"encoding/json"
	"fmt"
)

// DeleteCloudStackResource deletes an unmanaged CloudStack resource based on
// a raw JSON payload containing `kind` and `name` (or metadata.name).
// This wrapper is for standalone CLI mode.
func DeleteCloudStackResource(raw []byte) (string, error) {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", fmt.Errorf("invalid resource JSON: %w", err)
	}

	kind, _ := meta["kind"].(string)
	project, _ := meta["project"].(string)
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
		return "", fmt.Errorf("missing name for delete operation")
	}

	switch kind {
	case "VirtualMachine":
		id, err := DeleteVM(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Network":
		id, err := DeleteNetwork(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Volume":
		id, err := DeleteVolume(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "SSHKey":
		id, err := DeleteSSHKey(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "SecurityGroup":
		id, err := DeleteSecurityGroup(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "AffinityGroup":
		id, err := DeleteAffinityGroup(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Template":
		id, err := DeleteTemplate(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Snapshot":
		id, err := DeleteSnapshot(name)
		if err != nil {
			return "", err
		}
		return id, nil
	case "UserData":
		id, err := DeleteUserData(name, project)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Project":
		id, err := DeleteProject(name)
		if err != nil {
			return "", err
		}
		return id, nil
	default:
		return "", fmt.Errorf("unsupported resource kind for standalone delete: %s", kind)
	}
}
