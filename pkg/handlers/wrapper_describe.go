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
		return DescribeVM(name)
	case "Network":
		return DescribeNetwork(name)
	case "Template":
		return DescribeTemplate(name)
	case "Volume":
		return DescribeVolume(name)
	case "SSHKey":
		return DescribeSSHKey(name)
	case "SecurityGroup":
		return DescribeSecurityGroup(name)
	case "AffinityGroup":
		return DescribeAffinityGroup(name)
	case "UserData":
		return DescribeUserData(name)
	case "Snapshot":
		return DescribeSnapshot(name)
	default:
		return nil, fmt.Errorf("unsupported resource kind for standalone describe: %s", kind)
	}
}
