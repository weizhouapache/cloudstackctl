package handlers

import (
	"encoding/json"
	"fmt"
)

// DescribeCloudStackResource describes an unmanaged CloudStack resource based
// on a raw JSON payload containing `kind` and `name` (or metadata.name).
// This wrapper is for standalone CLI mode.
func DescribeCloudStackResource(raw []byte) (any, error) {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("invalid resource JSON: %w", err)
	}

	kind, _ := meta["kind"].(string)
	project, _ := meta["project"].(string)
	allProjects, _ := meta["allProjects"].(bool)
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
		return nil, fmt.Errorf("missing name for describe operation")
	}

	switch kind {
	case "VirtualMachine":
		return DescribeVM(name, project, allProjects)
	case "Network":
		return DescribeNetwork(name, project, allProjects)
	case "Template":
		return DescribeTemplate(name, project, allProjects)
	case "Volume":
		return DescribeVolume(name, project, allProjects)
	case "SSHKey":
		return DescribeSSHKey(name, project, allProjects)
	case "SecurityGroup":
		return DescribeSecurityGroup(name, project, allProjects)
	case "AffinityGroup":
		return DescribeAffinityGroup(name, project, allProjects)
	case "UserData":
		return DescribeUserData(name, project, allProjects)
	case "Snapshot":
		return DescribeSnapshot(name)
	case "Project":
		return DescribeProject(name)
	default:
		return nil, fmt.Errorf("unsupported resource kind for standalone describe: %s", kind)
	}
}
