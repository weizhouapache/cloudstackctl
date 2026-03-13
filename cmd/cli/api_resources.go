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
			{Name: "virtualmachines", ShortNames: "vm,vms", APIVersion: v1.APIVersion, Managed: false, Kind: "VirtualMachine"},
			{Name: "virtualmachinespecs", ShortNames: "vmspec,vmspecs", APIVersion: v1.APIVersion, Managed: true, Kind: "VirtualMachineSpec"},
			{Name: "networks", ShortNames: "net,nets,network,networks", APIVersion: v1.APIVersion, Managed: false, Kind: "Network"},
			{Name: "volumes", ShortNames: "vol,vols,volume,volumes", APIVersion: v1.APIVersion, Managed: false, Kind: "Volume"},
			{Name: "sshkeys", ShortNames: "key,keys,sshkey,sshkeys", APIVersion: v1.APIVersion, Managed: false, Kind: "SSHKey"},
			{Name: "userdata", ShortNames: "userdata,ud,uds", APIVersion: v1.APIVersion, Managed: false, Kind: "UserData"},
			{Name: "affinitygroups", ShortNames: "ag,affinitygroup,affinitygroups", APIVersion: v1.APIVersion, Managed: false, Kind: "AffinityGroup"},
			{Name: "securitygroups", ShortNames: "sg,sgs,securitygroup,securitygroups", APIVersion: v1.APIVersion, Managed: false, Kind: "SecurityGroup"},
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSHORTNAMES\tAPIVERSION\tManaged\tKIND")

		for _, r := range resources {
			fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n",
				r.Name, r.ShortNames, r.APIVersion, r.Managed, r.Kind)
		}

		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(apiResourcesCmd)
}
