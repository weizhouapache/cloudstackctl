package cli

import (
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"log"
	"net/url"

	"github.com/spf13/cobra"
)

// describeCmd represents the describe command
var describeCmd = &cobra.Command{
	Use:   "describe <resource-type> <name>",
	Short: "Show detailed information about a resource",
	Long:  `Show detailed information about a CloudStack resource managed by cloudstackctl`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 2 {
			log.Fatal("Usage: cloudstackctl describe <resource-type> <name>")
		}

		resourceType := normalizeResourceType(args[0])
		name := args[1]

		// Certain managed kinds are only supported via the controller
		if standalone {
			if resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachineSpec" {
				log.Fatalf("'%s' is not supported in standalone mode", resourceType)
			}
			// Standalone: use local describe wrapper
			payload := map[string]string{"kind": resourceType, "name": name}
			raw, _ := json.Marshal(payload)
			if err := handlers.DescribeCloudStackResource(raw); err != nil {
				log.Fatalf("Local describe failed: %v", err)
			}
			return
		}

		// Cluster mode: query controller describe endpoint
		path := "/describe?kind=" + url.QueryEscape(resourceType) + "&name=" + url.QueryEscape(name)
		body, err := ControllerRequest("GET", path, nil)
		if err != nil {
			log.Fatalf("Failed to query controller: %v", err)
		}
		println(string(body))
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
