package cli

import (
	"encoding/json"

	"log"
	"os"

	"cloudstackctl/pkg/handlers"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
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

		// Convert YAML to JSON for the API
		jsonData, err := yaml.YAMLToJSON(data)
		if err != nil {
			log.Fatalf("Failed to convert YAML to JSON: %v", err)
		}

		// Inspect kind
		var meta map[string]interface{}
		if err := json.Unmarshal(jsonData, &meta); err != nil {
			log.Fatalf("Invalid resource JSON: %v", err)
		}

		// standalone mode: apply the resource directly via handlers
		if standalone {
			// Application, Component, VirtualMachineSpec are only supported in controller mode
			kind, _ := meta["kind"].(string)
			if kind == "Application" || kind == "Component" || kind == "VirtualMachineSpec" {
				log.Fatalf("%s is not supported in standalone mode", kind)
			}
			if err := handlers.ApplyCloudStackResource(jsonData); err != nil {
				log.Fatalf("Local apply failed: %v", err)
			}
			return
		}

		// controller mode: apply by POSTing to controller HTTP API
		body, err := ControllerRequest("POST", "/apply", jsonData)
		if err != nil {
			log.Fatalf("Failed to POST to controller: %v", err)
		}
		log.Println("Resource accepted by controller:", string(body))
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	// Flags for apply command
	applyCmd.Flags().StringP("file", "f", "", "Path to YAML configuration file (required)")
	applyCmd.MarkFlagRequired("file")
}
