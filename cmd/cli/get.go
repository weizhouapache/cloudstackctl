package cli

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"log"
	"net/url"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"

	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get <resource-type> [name]",
	Short: "List CloudStack resources",
	Long:  `List resources managed by cloudstackctl (Application/Component/VirtualMachine/Network/Volume/etc.)`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			log.Fatal("Usage: cloudstackctl get <resource-type> [name]")
		}

		resourceType := normalizeResourceType(args[0])
		name := ""
		if len(args) > 1 {
			name = args[1]
		}

		// Standalone: call local handler wrapper
		if standalone {
			if resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachineSpec" {
				log.Fatalf("'%s' is not supported in standalone mode", resourceType)
			}

			payload := map[string]string{"kind": resourceType}
			if name != "" {
				payload["name"] = name
			}
			raw, _ := json.Marshal(payload)
			if resp, err := handlers.GetCloudStackResource(raw); err != nil {
				log.Fatalf("Local get failed: %v", err)
			} else {
				handlers.PrintCloudStackResource(resourceType, resp)
			}
			return
		}

		// Cluster mode: forward to controller HTTP API (/list)
		var endpoint = "/list"
		q := url.Values{}
		if getAll && resourceType == "VirtualMachine" {
			q.Set("all", "true")
		}
		q.Set("kind", resourceType)
		if name != "" {
			q.Set("name", name)
		}
		path := endpoint + "?" + q.Encode()
		body, err := ControllerRequest("GET", path, nil)
		if err != nil {
			log.Fatalf("Failed to query controller: %v", err)
		}

		// If controller returned VM objects from DB, pretty-print them
		if resourceType == "VirtualMachine" && !getAll {
			var vms []v1.VirtualMachine
			if err := json.Unmarshal(body, &vms); err == nil {
				handlers.PrintVMsFromDB(vms)
				return
			}
		}

		tryDecodeAndPrint(resourceType, body)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}

var getAll bool

func init() {
	getCmd.Flags().BoolVarP(&getAll, "all", "A", false, "Show all VMs from CloudStack (include unmanaged) — controller mode only")
}

// tryDecodeAndPrint attempts to decode controller JSON into known typed
// responses and prints them using the shared handlers. Returns true if
// printing was performed.
func tryDecodeAndPrint(resourceType string, body []byte) bool {
	// Try CloudStack SDK response types for unmanaged resources so we can
	// preserve typed slices (e.g., []*cs.Network) when printing.
	switch resourceType {
	case "VirtualMachine":
		var resp cs.ListVirtualMachinesResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "Network":
		var resp cs.ListNetworksResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "Volume":
		var resp cs.ListVolumesResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "Template":
		var resp cs.ListTemplatesResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "SSHKey":
		var resp cs.ListSSHKeyPairsResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "SecurityGroup":
		var resp cs.ListSecurityGroupsResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "AffinityGroup":
		var resp cs.ListAffinityGroupsResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	case "UserData":
		var resp cs.ListUserDataResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	}
	return false
}
