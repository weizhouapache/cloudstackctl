package cli

import (
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"log"
	"net/url"

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
			if err := handlers.GetCloudStackResource(raw); err != nil {
				log.Fatalf("Local get failed: %v", err)
			}
			return
		}

		// Cluster mode: forward to controller HTTP API (/list)
		var endpoint = "/list"
		q := url.Values{}
		q.Set("kind", resourceType)
		if name != "" {
			q.Set("name", name)
		}
		path := endpoint + "?" + q.Encode()
		body, err := ControllerRequest("GET", path, nil)
		if err != nil {
			log.Fatalf("Failed to query controller: %v", err)
		}
		println(string(body))
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
