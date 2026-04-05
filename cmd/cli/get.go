package cli

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

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
			log.Fatal("Usage: cloudstackctl get <resource-type>[,...] [name]")
		}

		// Support comma-separated resource types: e.g. "apps,comps,vms"
		rawKinds := strings.Split(args[0], ",")
		kinds := make([]string, 0, len(rawKinds))
		for _, k := range rawKinds {
			kinds = append(kinds, normalizeResourceType(k))
		}

		name := ""
		if len(args) > 1 {
			name = args[1]
		}

		// Standalone: call local handler wrapper. If any requested kind is
		// managed by the controller, fail in standalone mode.
		if standalone {
			if getAllVMs {
				log.Fatal("--all-vms/-A is supported in controller mode only")
			}
			for _, resourceType := range kinds {
				if resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachineSpec" {
					log.Fatalf("'%s' is not supported in standalone mode", resourceType)
				}
			}
			for i, resourceType := range kinds {
				payload := map[string]interface{}{"kind": resourceType}
				if name != "" {
					payload["name"] = name
				}
				if getProject != "" {
					payload["project"] = getProject
				}
				if getAllProjects {
					payload["allProjects"] = true
				}
				raw, _ := json.Marshal(payload)
				if resp, err := handlers.GetCloudStackResource(raw); err != nil {
					log.Fatalf("Local get failed: %v", err)
				} else {
					if i > 0 {
						fmt.Printf("\n")
					}
					handlers.PrintCloudStackResource(resourceType, resp)
				}
			}
			return
		}

		// Controller mode: query controller for each requested kind and print
		// results sequentially with simple headers.
		for i, resourceType := range kinds {
			var endpoint = "/list"
			q := url.Values{}
			q.Set("kind", resourceType)
			if name != "" {
				q.Set("name", name)
			}
			if getProject != "" {
				q.Set("project", getProject)
			}
			if getAllProjects {
				q.Set("all-projects", "true")
			}
			if getAllVMs && resourceType == "VirtualMachine" {
				q.Set("all-vms", "true")
			}
			if getApplication != "" && (resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachine") {
				q.Set("application", getApplication)
			}
			path := endpoint + "?" + q.Encode()
			body, err := ControllerRequest("GET", path, nil)
			if err != nil {
				log.Fatalf("Failed to query controller for %s: %v", resourceType, err)
			}

			if i > 0 {
				fmt.Printf("\n")
			}

			// DB-backed VMs are shown in controller mode unless CloudStack-scoped
			// VM listing is explicitly requested.
			if resourceType == "VirtualMachine" && !getAllVMs && getProject == "" && !getAllProjects {
				var vms []v1.VirtualMachine
				if err := json.Unmarshal(body, &vms); err == nil {
					handlers.PrintVMsFromController(vms)
					continue
				}
			}

			if tryDecodeAndPrint(resourceType, body) {
				continue
			}

			// Controller may return DB-backed resources; print them in-table when possible
			handled := false
			switch resourceType {
			case "Component":
				var comps []v1.Component
				if err := json.Unmarshal(body, &comps); err == nil {
					handlers.PrintComponents(comps)
					handled = true
				}
			case "VirtualMachineSpec":
				var specs []v1.VirtualMachineSpecResource
				if err := json.Unmarshal(body, &specs); err == nil {
					handlers.PrintVMSpecs(specs)
					handled = true
				}
			case "Application":
				var apps []v1.Application
				if err := json.Unmarshal(body, &apps); err == nil {
					handlers.PrintApplications(apps)
					handled = true
				}
			}
			if handled {
				continue
			}

			// Fallback: print raw body
			fmt.Println(string(body))
		}
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}

var getAllVMs bool
var getAllProjects bool
var getProject string
var getApplication string

func init() {
	getCmd.Flags().BoolVarP(&getAllVMs, "all-vms", "A", false, "Show all VMs from CloudStack (include unmanaged) - controller mode only")
	getCmd.Flags().BoolVarP(&getAllProjects, "all-projects", "P", false, "List resources across all projects (and no project)")
	getCmd.Flags().StringVarP(&getProject, "project", "p", "", "Filter results by project name")
	getCmd.Flags().StringVarP(&getApplication, "application", "a", "", "Filter Application/Component/VirtualMachine results by application name")
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
	case "Project":
		var resp cs.ListProjectsResponse
		if err := json.Unmarshal(body, &resp); err == nil {
			handlers.PrintCloudStackResource(resourceType, &resp)
			return true
		}
	}
	return false
}
