package cli

import (
	"bytes"
	"encoding/json"
	"io"

	"log"
	"net/http"
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
		kind, _ := meta["kind"].(string)

		// Decide whether resource is managed (controller) or unmanaged (local handlers).
		// Managed kinds are stored/controlled by the controller and should be
		// POSTed to it; unmanaged kinds are handled locally by handlers.
		isManaged := func(kind string) bool {
			switch kind {
			case "Application", "Component", "VirtualMachineSpec":
				return true
			default:
				return false
			}
		}

		// No per-VM managed detection here: standalone VMs are created locally;
		// in cluster mode VMs are treated as managed and POSTed to the controller.

		// Decide execution path based on standalone flag and managed status.
		// local apply helper used in standalone and for unmanaged cluster-mode resources
		applyLocal := func() {
			if err := handlers.ApplyCloudStackResource(jsonData); err != nil {
				log.Fatalf("Local apply failed: %v", err)
			}
		}

		if standalone {
			applyLocal()
			return
		}

		// Non-standalone (cluster) mode: managed resources go to controller.
		// Treat any VM as managed in cluster mode so controller can reconcile it.
		if isManaged(kind) || kind == "VirtualMachine" {
			server := os.Getenv("CONTROLLER_ENDPOINT")
			if server == "" {
				server = "http://localhost:65426"
			}
			url := server + "/apply"

			resp, err := http.Post(url, "application/json", bytes.NewReader(jsonData))
			if err != nil {
				log.Fatalf("Failed to POST to controller: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 300 {
				log.Fatalf("Controller returned %d: %s", resp.StatusCode, string(body))
			}

			log.Println("Resource accepted by controller:", string(body))
			return
		}

		// Unmanaged resource in cluster mode: apply locally via shared helper
		applyLocal()
		return
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)

	// Flags for apply command
	applyCmd.Flags().StringP("file", "f", "", "Path to YAML configuration file (required)")
	applyCmd.MarkFlagRequired("file")
}
