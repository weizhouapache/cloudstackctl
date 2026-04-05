package cli

import (
	"bytes"
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete <resource-type> <name> | delete -f <yaml>",
	Short: "Delete a CloudStack resource",
	Long:  `Delete a resource managed by cloudstackctl (Application/Component/VirtualMachine/etc.)`,
	Run: func(cmd *cobra.Command, args []string) {
		// file flag support (delete -f <yaml>)
		filePath, _ := cmd.Flags().GetString("file")
		projectFlag, _ := cmd.Flags().GetString("project")
		var name string
		var resourceType string
		if filePath != "" {
			var data []byte
			var err error
			data, err = os.ReadFile(filePath)
			if err != nil {
				log.Fatalf("Failed to read file: %v", err)
			}
			// Support multi-document YAML: decode each document and delete in reverse order
			dec := yaml.NewDecoder(bytes.NewReader(data))
			var docs []map[string]interface{}
			for {
				var doc map[string]interface{}
				if err := dec.Decode(&doc); err != nil {
					if err == io.EOF {
						break
					}
					log.Fatalf("Failed to decode YAML: %v", err)
				}
				if doc == nil {
					continue
				}
				docs = append(docs, doc)
			}
			if len(docs) == 0 {
				log.Fatalf("no resources found in YAML")
			}

			// Iterate in reverse order for deletion
			for i := len(docs) - 1; i >= 0; i-- {
				meta := docs[i]
				kind, _ := meta["kind"].(string)
				name = ""
				project := ""
				if m, ok := meta["metadata"].(map[string]interface{}); ok {
					if n, ok := m["name"].(string); ok {
						name = n
					}
					if p, ok := m["project"].(string); ok {
						project = p
					}
				}
				if project == "" && projectFlag != "" {
					project = projectFlag
				}
				if kind == "" || name == "" {
					log.Fatalf("each YAML doc must contain kind and metadata.name")
				}
				resourceType = kind
				payload := map[string]interface{}{"kind": resourceType, "name": name}
				if project != "" {
					payload["project"] = project
				}
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
					continue
				}

				// Cluster mode: send delete request to controller for each resource
				if body, err := ControllerRequest("POST", "/delete", rawPayload); err != nil {
					log.Printf("Controller delete failed for %s/%s: %v", resourceType, name, err)
				} else {
					// Remove redundant kind/name from returned JSON
					var obj map[string]interface{}
					if err := json.Unmarshal(body, &obj); err == nil {
						delete(obj, "kind")
						delete(obj, "name")
						if b2, err := json.Marshal(obj); err == nil {
							log.Printf("Controller response for %s/%s: %s", resourceType, name, string(b2))
							continue
						}
					}
					log.Printf("Controller response for %s/%s: %s", resourceType, name, string(body))
				}
			}
			return
		}

		if len(args) < 2 {
			log.Fatal("Usage: cloudstackctl delete <resource-type> <name>")
		}

		resourceType = normalizeResourceType(args[0])
		name = args[1]

		payload := map[string]interface{}{"kind": resourceType, "name": name}
		if projectFlag != "" {
			payload["project"] = projectFlag
		}
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
			// Remove redundant kind/name from returned JSON
			var obj map[string]interface{}
			if err := json.Unmarshal(body, &obj); err == nil {
				delete(obj, "kind")
				delete(obj, "name")
				if b2, err := json.Marshal(obj); err == nil {
					log.Printf("Controller response for %s/%s: %s", resourceType, name, string(b2))
					return
				}
			}
			log.Printf("Controller response for %s/%s: %s", resourceType, name, string(body))
		}
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().StringP("file", "f", "", "Path to YAML configuration file to delete")
	deleteCmd.Flags().StringP("project", "p", "", "Delete resource within a specific CloudStack project")
}
