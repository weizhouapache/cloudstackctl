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
func ApplyCloudStackResource(raw []byte) error {
	var meta map[string]interface{}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("invalid resource JSON: %w", err)
	}
	kind, _ := meta["kind"].(string)

	switch kind {
	case "VirtualMachine":
		var vm v1.VirtualMachine
		if err := json.Unmarshal(raw, &vm); err != nil {
			return fmt.Errorf("failed to parse VirtualMachine: %w", err)
		}
		return ApplyVirtualMachine(&vm)
	case "Network":
		var net v1.Network
		if err := json.Unmarshal(raw, &net); err != nil {
			return fmt.Errorf("failed to parse Network: %w", err)
		}
		return ApplyNetwork(&net)
	case "Volume":
		var vol v1.Volume
		if err := json.Unmarshal(raw, &vol); err != nil {
			return fmt.Errorf("failed to parse Volume: %w", err)
		}
		return ApplyVolume(&vol)
	case "SSHKey":
		var key v1.SSHKey
		if err := json.Unmarshal(raw, &key); err != nil {
			return fmt.Errorf("failed to parse SSHKey: %w", err)
		}
		return ApplySSHKey(&key)
	case "SecurityGroup":
		var sg v1.SecurityGroup
		if err := json.Unmarshal(raw, &sg); err != nil {
			return fmt.Errorf("failed to parse SecurityGroup: %w", err)
		}
		return ApplySecurityGroup(&sg)
	case "AffinityGroup":
		var ag v1.AffinityGroup
		if err := json.Unmarshal(raw, &ag); err != nil {
			return fmt.Errorf("failed to parse AffinityGroup: %w", err)
		}
		return ApplyAffinityGroup(&ag)
	case "UserData":
		var ud v1.UserData
		if err := json.Unmarshal(raw, &ud); err != nil {
			return fmt.Errorf("failed to parse UserData: %w", err)
		}
		return ApplyUserData(&ud)
	default:
		return fmt.Errorf("unsupported resource kind for cloudstack apply: %s", kind)
	}
}
