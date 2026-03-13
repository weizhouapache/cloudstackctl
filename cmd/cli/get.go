package cli

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/handlers"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get <resource-type>",
	Short: "List CloudStack resources",
	Long:  `List resources managed by cloudstackctl (Application/Component/VirtualMachine/Network/Volume/etc.)`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			log.Fatal("Please specify a resource type (Application/Component/VirtualMachine/Network/Volume/SSHKey/SecurityGroup/AffinityGroup/UserData)")
		}

		resourceType := normalizeResourceType(args[0])
		switch resourceType {
		case "Application":
			listApplications()
		case "Component":
			listComponents()
		case "VirtualMachine":
			listVMs()
		case "Template":
			if err := handlers.ListTemplates(); err != nil {
				log.Fatalf("Failed to list templates: %v", err)
			}
			return
		case "Volume":
			if err := handlers.ListVolumes(); err != nil {
				log.Fatalf("Failed to list volumes: %v", err)
			}
			return
		case "SSHKey":
			if err := handlers.ListSSHKeys(); err != nil {
				log.Fatalf("Failed to list ssh keys: %v", err)
			}
			return
		case "UserData":
			if err := handlers.ListUserData(); err != nil {
				log.Fatalf("Failed to list userdata: %v", err)
			}
			return
		case "AffinityGroup":
			if err := handlers.ListAffinityGroups(); err != nil {
				log.Fatalf("Failed to list affinity groups: %v", err)
			}
			return
		case "SecurityGroup":
			if err := handlers.ListSecurityGroups(); err != nil {
				log.Fatalf("Failed to list security groups: %v", err)
			}
			return
		case "VirtualMachineSpec":
			log.Fatalf("'VirtualMachineSpec' listing is not implemented")
		case "Network":
			if err := handlers.ListNetworks(); err != nil {
				log.Fatalf("Failed to list networks: %v", err)
			}
		default:
			log.Fatalf("Unsupported resource type: %s", args[0])
		}
	},
}

// normalizeResourceType maps shortnames to canonical resource type names
func normalizeResourceType(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	switch lower {
	case "app", "apps", "application", "applications":
		return "Application"
	case "comp", "comps", "component", "components":
		return "Component"
	case "vm", "vms", "virtualmachine", "virtualmachines":
		return "VirtualMachine"
	case "vmspec", "vmspecs", "virtualmachinespec", "virtualmachinespecs":
		return "VirtualMachineSpec"
	case "net", "nets", "network", "networks":
		return "Network"
	case "vol", "vols", "volume", "volumes":
		return "Volume"
	case "key", "keys", "sshkey", "sshkeys":
		return "SSHKey"
	case "userdata", "ud", "uds":
		return "UserData"
	case "ag", "affinitygroup", "affinitygroups":
		return "AffinityGroup"
	case "sg", "sgs", "securitygroup", "securitygroups":
		return "SecurityGroup"
	default:
		return s
	}
}

// listNetworks lists Network resources (delegates to handlers)
func listNetworks() {
	if err := handlers.ListNetworks(); err != nil {
		log.Fatalf("Failed to list networks: %v", err)
	}
}

func init() {
	rootCmd.AddCommand(getCmd)
}

// listApplications lists all Application resources
func listApplications() {
	// Do not use DB in standalone mode
	if standalone {
		log.Fatalf("'Application' is not supported in standalone mode")
	}

	// Ensure DB initialized if possible
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Printf("Database unavailable: %v", err)
			return
		}
	}

	var apps []v1.Application
	if err := db.DB.Find(&apps).Error; err != nil {
		log.Fatalf("Failed to list applications: %v", err)
	}

	// Create tab writer for formatted output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROJECT ID\tSTATUS\tREADY\tDRIFT")

	for _, app := range apps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\n",
			app.Metadata.Name,
			app.Spec.ProjectID,
			app.Status.ObservedState,
			app.Status.Ready,
			app.Status.Drift,
		)
	}

	w.Flush()
}

// listComponents lists all Component resources
func listComponents() {
	// Component not supported in standalone mode
	if standalone {
		log.Fatalf("'Component' is not supported in standalone mode")
	}

	// Ensure DB initialized if possible
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Printf("Database unavailable: %v", err)
			return
		}
	}

	var comps []v1.Component
	if err := db.DB.Find(&comps).Error; err != nil {
		log.Fatalf("Failed to list components: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tREPLICAS\tSTATUS\tREADY\tDRIFT")

	for _, comp := range comps {
		fmt.Fprintf(w, "%s\t%d\t%s\t%t\t%t\n",
			comp.Metadata.Name,
			comp.Spec.Replicas,
			comp.Status.ObservedState,
			comp.Status.Ready,
			comp.Status.Drift,
		)
	}

	w.Flush()
}

// listVMs lists all VirtualMachine resources
func listVMs() {
	// If running standalone, always query CloudStack directly
	if standalone {
		// No DB: query CloudStack API for VMs
		if err := handlers.ListVMs(); err != nil {
			log.Fatalf("Failed to list VMs: %v", err)
		}
		return
	}

	// If DB is available, list from DB; otherwise query CloudStack directly
	if db.DB == nil {
		// try to initialize DB; in cluster mode we do NOT query CloudStack directly
		if err := db.Init(); err != nil {
			log.Printf("Database unavailable: %v", err)
			log.Printf("Run with -s/--standalone to query CloudStack directly")
			return
		}
	}

	if db.DB != nil {
		var vms []v1.VirtualMachine
		if err := db.DB.Find(&vms).Error; err != nil {
			log.Fatalf("Failed to list VMs: %v", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tID\tTEMPLATE\tSERVICE OFFERING\tSTATUS\tREADY\tDRIFT")

		for _, vm := range vms {
			id := vm.Status.CloudStackID
			tmpl := vm.Spec.Template
			if tmpl == "" && vm.ObservedSpec.Template != "" {
				tmpl = vm.ObservedSpec.Template
			}
			so := vm.Spec.ServiceOffering
			if so == "" && vm.ObservedSpec.ServiceOffering != "" {
				so = vm.ObservedSpec.ServiceOffering
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\t%t\n",
				vm.Metadata.Name,
				id,
				tmpl,
				so,
				vm.Status.ObservedState,
				vm.Status.Ready,
				vm.Status.Drift,
			)
		}

		w.Flush()
	}
}
