package cli

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/cloudstack"
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"log"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete <resource-type> <name>",
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
			// proceed with delete
			switch resourceType {
			case "Application":
				deleteApplication(name)
			case "Component":
				deleteComponent(name)
			case "VirtualMachine":
				deleteVM(name)
			case "Network":
				if err := handlers.DeleteNetwork(name); err != nil {
					log.Fatalf("Failed to delete network: %v", err)
				}
			default:
				log.Fatalf("Unsupported resource type: %s", resourceType)
			}
			return
		}

		if len(args) < 2 {
			log.Fatal("Usage: cloudstackctl delete <resource-type> <name>")
		}

		resourceType = args[0]
		name = args[1]

		// Delete resource based on type
		switch resourceType {
		case "Application":
			deleteApplication(name)
		case "Component":
			deleteComponent(name)
		case "VirtualMachine":
			deleteVM(name)
		case "Network":
			if err := handlers.DeleteNetwork(name); err != nil {
				log.Fatalf("Failed to delete network: %v", err)
			}
		case "Volume":
			if err := handlers.DeleteVolume(name); err != nil {
				log.Fatalf("Failed to delete volume: %v", err)
			}
			return
		case "SSHKey":
			if err := handlers.DeleteSSHKey(name); err != nil {
				log.Fatalf("Failed to delete ssh key: %v", err)
			}
			return
		case "SecurityGroup":
			if err := handlers.DeleteSecurityGroup(name); err != nil {
				log.Fatalf("Failed to delete security group: %v", err)
			}
			return
		case "Template":
			if err := handlers.DeleteTemplate(name); err != nil {
				log.Fatalf("Failed to delete template: %v", err)
			}
			return
		default:
			log.Fatalf("Unsupported resource type: %s", resourceType)
		}
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().StringP("file", "f", "", "Path to YAML configuration file to delete")
}

// deleteApplication deletes an Application and its dependent resources
func deleteApplication(name string) {
	var app v1.Application
	if err := db.DB.Where("metadata_name = ?", name).First(&app).Error; err != nil {
		log.Fatalf("Application %s not found: %v", name, err)
	}

	// Delete dependent components first
	for _, compRef := range app.Spec.Components {
		deleteComponent(compRef.Name)
	}

	// Delete application from database
	if err := db.DB.Delete(&app).Error; err != nil {
		log.Fatalf("Failed to delete application %s: %v", name, err)
	}

	log.Printf("Application %s deleted successfully", name)
}

// deleteComponent deletes a Component and its VMs
func deleteComponent(name string) {
	var comp v1.Component
	if err := db.DB.Where("metadata_name = ?", name).First(&comp).Error; err != nil {
		log.Fatalf("Component %s not found: %v", name, err)
	}

	// Delete component VMs
	var vms []v1.VirtualMachine
	if err := db.DB.Where("metadata_labels @> ?", map[string]string{"component": name}).Find(&vms).Error; err != nil {
		log.Fatalf("Failed to find VMs for component %s: %v", name, err)
	}

	for _, vm := range vms {
		deleteVM(vm.Metadata.Name)
	}

	// Delete component from database
	if err := db.DB.Delete(&comp).Error; err != nil {
		log.Fatalf("Failed to delete component %s: %v", name, err)
	}

	log.Printf("Component %s deleted successfully", name)
}

// deleteVM deletes a VM from CloudStack and database
func deleteVM(name string) {
	var vm v1.VirtualMachine
	if err := db.DB.Where("metadata_name = ?", name).First(&vm).Error; err != nil {
		log.Fatalf("VM %s not found: %v", name, err)
	}

	// Delete VM from CloudStack if exists
	if vm.Status.CloudStackID != "" {
		csClient, err := cloudstack.NewClient()
		if err != nil {
			log.Printf("Warning: CloudStack client unavailable, skipping CloudStack delete: %v", err)
		} else {
			params := csClient.VirtualMachine.NewDestroyVirtualMachineParams(vm.Status.CloudStackID)
			_, err := csClient.VirtualMachine.DestroyVirtualMachine(params)
			if err != nil {
				log.Printf("Warning: Failed to delete VM %s from CloudStack: %v", name, err)
			}
		}
	}

	// Delete VM from database
	if err := db.DB.Delete(&vm).Error; err != nil {
		log.Fatalf("Failed to delete VM %s from database: %v", name, err)
	}

	log.Printf("VM %s deleted successfully", name)
}

// deleteNetwork deletes a Network resource
func deleteNetwork(name string) {
	// If standalone mode, try to find and delete in CloudStack directly
	if standalone {
		cs, err := cloudstack.NewClient()
		if err != nil {
			log.Fatalf("CloudStack client unavailable: %v", err)
		}
		params := cs.Network.NewListNetworksParams()
		params.SetName(name)
		resp, err := cs.Network.ListNetworks(params)
		if err != nil {
			log.Fatalf("CloudStack network lookup failed: %v", err)
		}
		if resp == nil || len(resp.Networks) == 0 {
			log.Fatalf("Network %s not found in CloudStack", name)
		}
		nid := resp.Networks[0].Id
		delp := cs.Network.NewDeleteNetworkParams(nid)
		if _, err := cs.Network.DeleteNetwork(delp); err != nil {
			log.Fatalf("Failed to delete Network %s from CloudStack: %v", name, err)
		}
		log.Printf("Network %s deleted from CloudStack (id=%s)", name, nid)
		return
	}

	// Cluster mode: delete from DB and attempt CloudStack deletion if external ID present
	var n v1.Network
	if err := db.DB.Where("metadata_name = ?", name).First(&n).Error; err != nil {
		log.Fatalf("Network %s not found: %v", name, err)
	}

	if n.Status.CloudStackID != "" {
		cs, err := cloudstack.NewClient()
		if err != nil {
			log.Printf("Warning: CloudStack client unavailable, skipping external delete: %v", err)
		} else {
			delp := cs.Network.NewDeleteNetworkParams(n.Status.CloudStackID)
			if _, err := cs.Network.DeleteNetwork(delp); err != nil {
				log.Printf("Warning: Failed to delete network %s from CloudStack: %v", name, err)
			}
		}
	}

	if err := db.DB.Delete(&n).Error; err != nil {
		log.Fatalf("Failed to delete network %s from database: %v", name, err)
	}

	log.Printf("Network %s deleted successfully", name)
}
