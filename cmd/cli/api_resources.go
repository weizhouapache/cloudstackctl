package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	v1 "cloudstackctl/apis/v1"

	"github.com/spf13/cobra"
)

// apiResourcesCmd lists supported API resources and their shortnames
var apiResourcesCmd = &cobra.Command{
	Use:   "api-resources",
	Short: "List supported cloudstackctl API resources and shortnames",
	Run: func(cmd *cobra.Command, args []string) {
		type res struct {
			Name       string
			ShortNames string
			APIVersion string
			Managed    bool
			Kind       string
		}

		resources := []res{
			{Name: "applications", ShortNames: "app,apps", APIVersion: v1.APIVersion, Managed: true, Kind: "Application"},
			{Name: "components", ShortNames: "comp,comps", APIVersion: v1.APIVersion, Managed: true, Kind: "Component"},
			{Name: "virtualmachines", ShortNames: "vm,vms", APIVersion: v1.APIVersion, Managed: true, Kind: "VirtualMachine"},
			{Name: "virtualmachinespecs", ShortNames: "vmspec,vmspecs", APIVersion: v1.APIVersion, Managed: true, Kind: "VirtualMachineSpec"},
			{Name: "networks", ShortNames: "net,nets,network,networks", APIVersion: v1.APIVersion, Managed: false, Kind: "Network"},
			{Name: "volumes", ShortNames: "vol,vols,volume,volumes", APIVersion: v1.APIVersion, Managed: false, Kind: "Volume"},
			{Name: "sshkeys", ShortNames: "key,keys,sshkey,sshkeys", APIVersion: v1.APIVersion, Managed: false, Kind: "SSHKey"},
			{Name: "userdata", ShortNames: "userdata,ud,uds", APIVersion: v1.APIVersion, Managed: false, Kind: "UserData"},
			{Name: "affinitygroups", ShortNames: "ag,affinitygroup,affinitygroups", APIVersion: v1.APIVersion, Managed: false, Kind: "AffinityGroup"},
			{Name: "securitygroups", ShortNames: "sg,sgs,securitygroup,securitygroups", APIVersion: v1.APIVersion, Managed: false, Kind: "SecurityGroup"},
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSHORTNAMES\tAPIVERSION\tManaged\tSupported\tKIND")

		for _, r := range resources {
			supported := true
			if standalone {
				// In standalone mode, controller-managed resources are not supported
				// except VirtualMachine which is supported in both modes.
				switch r.Kind {
				case "VirtualMachine":
					supported = true
				case "Application", "Component", "VirtualMachineSpec":
					supported = false
				default:
					supported = true
				}
			}
			var suppStr string
			if supported {
				suppStr = "yes"
			} else {
				suppStr = "no"
			}

			// Managed column: show boolean in controller mode, "n/a" in standalone mode
			var managedStr string
			if standalone {
				managedStr = "n/a"
			} else if r.Managed {
				managedStr = "yes"
			} else {
				managedStr = "no"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.Name, r.ShortNames, r.APIVersion, managedStr, suppStr, r.Kind)
		}

		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(apiResourcesCmd)
}
