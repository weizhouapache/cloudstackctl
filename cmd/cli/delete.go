package cli

import (
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"log"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete <resource-type> <name> | delete -f <yaml>",
	Short: "Delete a CloudStack resource",
	Long:  `Delete a resource managed by cloudstackctl (Application/Component/VirtualMachine/etc.)`,
	Run: func(cmd *cobra.Command, args []string) {
		// file flag support (delete -f <yaml>)
		filePath, _ := cmd.Flags().GetString("file")
		var name string
		var jsonData []byte
		var resourceType string
		if filePath != "" {
			var data []byte
			var err error
			data, err = os.ReadFile(filePath)
			if err != nil {
				log.Fatalf("Failed to read file: %v", err)
			}
			jsonData, err = yaml.YAMLToJSON(data)
			if err != nil {
				log.Fatalf("Failed to convert YAML to JSON: %v", err)
			}
			var meta map[string]interface{}
			if err := json.Unmarshal(jsonData, &meta); err != nil {
				log.Fatalf("Invalid resource JSON: %v", err)
			}
			kind, _ := meta["kind"].(string)
			// metadata may be nested
			name = ""
			if m, ok := meta["metadata"].(map[string]interface{}); ok {
				if n, ok := m["name"].(string); ok {
					name = n
				}
			}
			if kind == "" || name == "" {
				log.Fatal("YAML must contain kind and metadata.name")
			}
			resourceType = kind
			payload := map[string]string{"kind": resourceType, "name": name}
			rawPayload, _ := json.Marshal(payload)

			if standalone {
				// Only unmanaged kinds supported in standalone
				if resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachineSpec" {
					log.Fatalf("'%s' is not supported in standalone mode", resourceType)
				}
				if id, err := handlers.DeleteCloudStackResource(rawPayload); err != nil {
					log.Fatalf("Local delete failed: %v", err)
				} else {
					if id != "" {
						log.Printf("Deleted %s id=%s", resourceType, id)
					}
				}
				return
			}

			// Cluster mode: send delete request to controller
			if body, err := ControllerRequest("POST", "/delete", rawPayload); err != nil {
				log.Fatalf("Controller delete failed: %v", err)
			} else {
				log.Printf("Controller response for %s: %s", resourceType, string(body))
			}
			return
		}

		if len(args) < 2 {
			log.Fatal("Usage: cloudstackctl delete <resource-type> <name>")
		}

		resourceType = normalizeResourceType(args[0])
		name = args[1]

		payload := map[string]string{"kind": resourceType, "name": name}
		rawPayload, _ := json.Marshal(payload)

		if standalone {
			if resourceType == "Application" || resourceType == "Component" || resourceType == "VirtualMachineSpec" {
				log.Fatalf("'%s' is not supported in standalone mode", resourceType)
			}
			if id, err := handlers.DeleteCloudStackResource(rawPayload); err != nil {
				log.Fatalf("Local delete failed: %v", err)
			} else {
				if id != "" {
					log.Printf("Deleted %s id=%s", resourceType, id)
				}
			}
			return
		}

		// Cluster mode: instruct controller to delete the resource
		if body, err := ControllerRequest("POST", "/delete", rawPayload); err != nil {
			log.Fatalf("Controller delete failed: %v", err)
		} else {
			log.Printf("Controller response for %s: %s", resourceType, string(body))
		}
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().StringP("file", "f", "", "Path to YAML configuration file to delete")
}
