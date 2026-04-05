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
func GetCloudStackResource(raw []byte) (any, error) {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("invalid resource JSON: %w", err)
	}

	kind, _ := meta["kind"].(string)
	project, _ := meta["project"].(string)
	allProjects, _ := meta["allProjects"].(bool)

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
		return ListVMs(name, project, allProjects)
	case "Network":
		return ListNetworks(name, project, allProjects)
	case "Template":
		return ListTemplates(name, project, allProjects)
	case "Volume":
		return ListVolumes(name, project, allProjects)
	case "SSHKey":
		return ListSSHKeys(name, project, allProjects)
	case "SecurityGroup":
		return ListSecurityGroups(name, project, allProjects)
	case "AffinityGroup":
		return ListAffinityGroups(name, project, allProjects)
	case "UserData":
		return ListUserData(name, project, allProjects)
	case "Project":
		return ListProjects(name)
	default:
		return nil, fmt.Errorf("unsupported resource kind for cloudstack get: %s", kind)
	}
}
