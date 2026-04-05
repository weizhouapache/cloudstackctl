package cli

import (
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"fmt"
	"log"
	"net/url"

	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:     "describe <resource-type> <name>",
	Aliases: []string{"desc"},
	Short:   "Show detailed information about a resource",
	Long:    `Show detailed information about a CloudStack resource managed by cloudstackctl`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 2 {
			log.Fatal("Usage: cloudstackctl describe <resource-type> <name>")
		}

		resourceType := normalizeResourceType(args[0])
		name := args[1]

		// Certain managed kinds are only supported via the controller
		if standalone {
			if describeAllVMs {
				log.Fatal("--all-vms/-A is supported in controller mode only")
			}
			if resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachineSpec" {
				log.Fatalf("'%s' is not supported in standalone mode", resourceType)
			}
			// Standalone: use local describe wrapper
			payload := map[string]interface{}{"kind": resourceType, "name": name}
			if describeProject != "" {
				payload["project"] = describeProject
			}
			raw, _ := json.Marshal(payload)
			if respAny, err := handlers.DescribeCloudStackResource(raw); err != nil {
				log.Fatalf("Local describe failed: %v", err)
			} else {
				// Try to marshal the returned object into pretty JSON
				if b, jerr := json.MarshalIndent(respAny, "", "  "); jerr == nil {
					fmt.Println(string(b))
				} else {
					// Fallbacks: if it's a []byte, print string; otherwise use default fmt
					if bs, ok := respAny.([]byte); ok {
						fmt.Println(string(bs))
					} else {
						fmt.Printf("%v\n", respAny)
					}
				}
			}
			return
		}

		// Cluster mode: query controller describe endpoint
		q := url.Values{}
		q.Set("kind", resourceType)
		q.Set("name", name)
		if describeProject != "" {
			q.Set("project", describeProject)
		}
		if describeAllVMs && resourceType == "VirtualMachine" {
			q.Set("all-vms", "true")
		}
		path := "/describe?" + q.Encode()
		body, err := ControllerRequest("GET", path, nil)
		if err != nil {
			log.Fatalf("Failed to query controller: %v", err)
		} else {
			var obj any
			if uerr := json.Unmarshal(body, &obj); uerr != nil {
				// Not valid JSON? print raw.
				fmt.Println(string(body))
			} else {
				if b, merr := json.MarshalIndent(obj, "", "  "); merr == nil {
					fmt.Println(string(b))
				} else {
					fmt.Println(string(body))
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

var describeAllVMs bool
var describeProject string

func init() {
	describeCmd.Flags().BoolVarP(&describeAllVMs, "all-vms", "A", false, "Describe a VM directly from CloudStack (controller mode only)")
	describeCmd.Flags().StringVarP(&describeProject, "project", "p", "", "Look up resource within a specific CloudStack project")
}
