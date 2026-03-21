package handlers

import (
	"encoding/json"
	"fmt"

	v1 "cloudstackctl/apis/v1"
)

// ApplyCloudStackResource applies an unmanaged CloudStack resource from raw
// JSON. It inspects the `kind` field and dispatches to the appropriate
// handler. This wrapper is intended to be used by both the CLI and the
// controller for resources that are applied directly to CloudStack.
func ApplyCloudStackResource(raw []byte) (string, error) {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", fmt.Errorf("invalid resource JSON: %w", err)
	}
	kind, _ := meta["kind"].(string)

	switch kind {
	case "VirtualMachine":
		var vm v1.VirtualMachine
		if err := json.Unmarshal(raw, &vm); err != nil {
			return "", fmt.Errorf("failed to parse VirtualMachine: %w", err)
		}
		id, err := ApplyVirtualMachine(&vm)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Network":
		var net v1.Network
		if err := json.Unmarshal(raw, &net); err != nil {
			return "", fmt.Errorf("failed to parse Network: %w", err)
		}
		id, err := ApplyNetwork(&net)
		if err != nil {
			return "", err
		}
		return id, nil
	case "Volume":
		var vol v1.Volume
		if err := json.Unmarshal(raw, &vol); err != nil {
			return "", fmt.Errorf("failed to parse Volume: %w", err)
		}
		id, err := ApplyVolume(&vol)
		if err != nil {
			return "", err
		}
		return id, nil
	case "SSHKey":
		var key v1.SSHKey
		if err := json.Unmarshal(raw, &key); err != nil {
			return "", fmt.Errorf("failed to parse SSHKey: %w", err)
		}
		id, err := ApplySSHKey(&key)
		if err != nil {
			return "", err
		}
		return id, nil
	case "SecurityGroup":
		var sg v1.SecurityGroup
		if err := json.Unmarshal(raw, &sg); err != nil {
			return "", fmt.Errorf("failed to parse SecurityGroup: %w", err)
		}
		id, err := ApplySecurityGroup(&sg)
		if err != nil {
			return "", err
		}
		return id, nil
	case "AffinityGroup":
		var ag v1.AffinityGroup
		if err := json.Unmarshal(raw, &ag); err != nil {
			return "", fmt.Errorf("failed to parse AffinityGroup: %w", err)
		}
		id, err := ApplyAffinityGroup(&ag)
		if err != nil {
			return "", err
		}
		return id, nil
	case "UserData":
		var ud v1.UserData
		if err := json.Unmarshal(raw, &ud); err != nil {
			return "", fmt.Errorf("failed to parse UserData: %w", err)
		}
		id, err := ApplyUserData(&ud)
		if err != nil {
			return "", err
		}
		return id, nil
	default:
		return "", fmt.Errorf("unsupported resource kind for cloudstack apply: %s", kind)
	}
}
