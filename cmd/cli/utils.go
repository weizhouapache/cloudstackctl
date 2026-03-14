package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// normalizeResourceType maps shortnames to canonical resource type names
func normalizeResourceType(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "app", "apps", "application", "applications":
		return "Application"
	case "comp", "comps", "component", "components":
		return "Component"
	case "vm", "vms", "virtualmachine", "virtualmachines":
		return "VirtualMachine"
	case "vmspec", "vmspecs", "virtualmachinespec", "virtualmachinespecs":
		return "VirtualMachineSpec"
	case "net", "nets", "network", "networks":
		return "Network"
	case "vol", "vols", "volume", "volumes":
		return "Volume"
	case "key", "keys", "sshkey", "sshkeys":
		return "SSHKey"
	case "userdata", "ud", "uds":
		return "UserData"
	case "ag", "affinitygroup", "affinitygroups":
		return "AffinityGroup"
	case "sg", "sgs", "securitygroup", "securitygroups":
		return "SecurityGroup"
	default:
		return s
	}
}

// ControllerRequest sends an HTTP request to the controller at the given path
// using the provided method (e.g. "POST", "GET", "DELETE"). The jsonData
// is used as the request body for methods that support a body (POST/DELETE).
func ControllerRequest(method, path string, jsonData []byte) ([]byte, error) {
	server := os.Getenv("CONTROLLER_ENDPOINT")
	if server == "" {
		server = "http://localhost:65426"
	}
	url := server + path

	var reqBody io.Reader
	if len(jsonData) > 0 && (method == "POST" || method == "DELETE" || method == "PUT") {
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return body, fmt.Errorf("controller returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
