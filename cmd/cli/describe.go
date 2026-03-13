package cli

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"log"

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

		// Describe resource based on type
		switch resourceType {
		case "Application":
			describeApplication(name)
		case "Component":
			describeComponent(name)
		case "VirtualMachine":
			if standalone {
				if err := handlers.DescribeVM(name); err != nil {
					log.Fatalf("Failed to describe network: %v", err)
				}
				return
			}
			describeVM(name)
		case "Network":
			if err := handlers.DescribeNetwork(name); err != nil {
				log.Fatalf("Failed to describe network: %v", err)
			}
			return
		case "Template":
			if err := handlers.DescribeTemplate(name); err != nil {
				log.Fatalf("Failed to describe template: %v", err)
			}
			return
		case "Volume":
			if err := handlers.DescribeVolume(name); err != nil {
				log.Fatalf("Failed to describe volume: %v", err)
			}
			return
		case "SSHKey":
			if err := handlers.DescribeSSHKey(name); err != nil {
				log.Fatalf("Failed to describe ssh key: %v", err)
			}
			return
		case "UserData":
			if err := handlers.DescribeUserData(name); err != nil {
				log.Fatalf("Failed to describe userdata: %v", err)
			}
			return
		case "SecurityGroup":
			if err := handlers.DescribeSecurityGroup(name); err != nil {
				log.Fatalf("Failed to describe security group: %v", err)
			}
			return
		case "AffinityGroup":
			if err := handlers.DescribeAffinityGroup(name); err != nil {
				log.Fatalf("Failed to describe affinity group: %v", err)
			}
			return
		default:
			log.Fatalf("Unsupported resource type: %s", resourceType)
		}
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

// describeApplication shows detailed info about an Application
func describeApplication(name string) {
	var app v1.Application
	if err := db.DB.Where("metadata_name = ?", name).First(&app).Error; err != nil {
		log.Fatalf("Application %s not found: %v", name, err)
	}

	// Pretty print as JSON
	data, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		log.Fatalf("Failed to format application data: %v", err)
	}

	log.Println(string(data))
}

// describeComponent shows detailed info about a Component
func describeComponent(name string) {
	var comp v1.Component
	if err := db.DB.Where("metadata_name = ?", name).First(&comp).Error; err != nil {
		log.Fatalf("Component %s not found: %v", name, err)
	}

	data, err := json.MarshalIndent(comp, "", "  ")
	if err != nil {
		log.Fatalf("Failed to format component data: %v", err)
	}

	log.Println(string(data))
}

// describeVM shows detailed info about a VirtualMachine
func describeVM(name string) {
	var vm v1.VirtualMachine
	if err := db.DB.Where("metadata_name = ?", name).First(&vm).Error; err != nil {
		log.Fatalf("VM %s not found: %v", name, err)
	}

	data, err := json.MarshalIndent(vm, "", "  ")
	if err != nil {
		log.Fatalf("Failed to format VM data: %v", err)
	}

	log.Println(string(data))
}
