package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"

	"cloudstackctl/pkg/handlers"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply -f <yaml-file>",
	Short: "Apply a YAML configuration to CloudStack",
	Long:  `Apply reads a YAML file and either posts to a running controller server or applies directly depending on resource kind.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get YAML file path from flag
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			log.Fatal("Please specify a YAML file with -f/--file")
		}

		// Read YAML file
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Failed to read file: %v", err)
		}

		// Determine kind from first document for logging and mode handling.
		dec := yaml.NewDecoder(bytes.NewReader(data))
		var first map[string]interface{}
		if err := dec.Decode(&first); err != nil && err != io.EOF {
			log.Fatalf("Failed to parse YAML: %v", err)
		}
		kind := ""
		if first != nil {
			if k, ok := first["kind"].(string); ok {
				kind = k
			}
		}

		// standalone mode: apply each document locally
		if standalone {
			dec := yaml.NewDecoder(bytes.NewReader(data))
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
				j, _ := json.Marshal(doc)
				// inspect kind and reject unsupported managed kinds
				kk, _ := doc["kind"].(string)
				if kk == "Application" || kk == "Component" || kk == "VirtualMachineSpec" {
					log.Fatalf("%s is not supported in standalone mode", kk)
				}
				if id, err := handlers.ApplyCloudStackResource(j); err != nil {
					log.Fatalf("Local apply failed for %s: %v", kk, err)
				} else {
					if id != "" {
						log.Printf("Applied %s id=%s", kk, id)
					}
				}
			}
			return
		}

		// controller mode: POST the raw file bytes (controller will decode multiple docs)
		body, err := ControllerRequest("POST", "/apply", data)
		if err != nil {
			log.Fatalf("Failed to POST to controller: %v", err)
		}
		// If controller returned a JSON array of per-resource results, print
		// a concise one-line summary per resource. Otherwise pretty-print.
		var arr []map[string]interface{}
		if err := json.Unmarshal(body, &arr); err == nil {
			// Remove redundant kind/name from returned JSON
			for _, item := range arr {
				k, _ := item["kind"].(string)
				name, _ := item["name"].(string)
				delete(item, "kind")
				delete(item, "name")
				b, _ := json.Marshal(item)
				log.Printf("Controller response for %s/%s: %s", k, name, string(b))
			}
		} else {
			var resp interface{}
			if err := json.Unmarshal(body, &resp); err != nil {
				log.Printf("Controller accepted %s: %s", kind, string(body))
			} else {
				pretty, _ := json.MarshalIndent(resp, "", "  ")
				log.Printf("Controller accepted %s:\n%s", kind, string(pretty))
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	// Flags for apply command
	applyCmd.Flags().StringP("file", "f", "", "Path to YAML configuration file (required)")
	applyCmd.MarkFlagRequired("file")
}
