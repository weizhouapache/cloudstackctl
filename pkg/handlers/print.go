package handlers

import (
	"cloudstackctl/pkg/cloudstack"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	v1 "cloudstackctl/apis/v1"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

// PrintCloudStackResource renders a short tabular view for common CloudStack
// list response objects. If the kind is not recognized or the provided
// object doesn't match expected SDK types, the function falls back to
// printing indented JSON.
func PrintCloudStackResource(kind string, obj any) error {
	switch kind {
	case "Volume":
		if resp, ok := obj.(*cs.ListVolumesResponse); ok {
			PrintVolumes(resp.Volumes)
			return nil
		}
	case "Network":
		if resp, ok := obj.(*cs.ListNetworksResponse); ok {
			PrintNetworks(resp.Networks)
			return nil
		}
	case "VirtualMachine":
		if resp, ok := obj.(*cs.ListVirtualMachinesResponse); ok {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tTEMPLATE\tSERVICE OFFERING\tSTATUS")
			for _, v := range resp.VirtualMachines {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Name, v.Id, v.Templatename, v.Serviceofferingname, v.State)
			}
			w.Flush()
			return nil
		}
	case "Template":
		if resp, ok := obj.(*cs.ListTemplatesResponse); ok {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tOS\tFEATURED")
			for _, t := range resp.Templates {
				fmt.Fprintf(w, "%s\t%s\t%s\t%t\n", t.Name, t.Id, t.Ostypename, t.Isfeatured)
			}
			w.Flush()
			return nil
		}
	case "SSHKey":
		if resp, ok := obj.(*cs.ListSSHKeyPairsResponse); ok {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tFINGERPRINT")
			for _, k := range resp.SSHKeyPairs {
				fmt.Fprintf(w, "%s\t%s\n", k.Name, k.Fingerprint)
			}
			w.Flush()
			return nil
		}
	case "SecurityGroup":
		if resp, ok := obj.(*cs.ListSecurityGroupsResponse); ok {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tDESCRIPTION")
			for _, sg := range resp.SecurityGroups {
				fmt.Fprintf(w, "%s\t%s\t%s\n", sg.Name, sg.Id, sg.Description)
			}
			w.Flush()
			return nil
		}
	case "AffinityGroup":
		if resp, ok := obj.(*cs.ListAffinityGroupsResponse); ok {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tDESCRIPTION")
			for _, a := range resp.AffinityGroups {
				fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, a.Id, a.Description)
			}
			w.Flush()
			return nil
		}
	case "UserData":
		if resp, ok := obj.(*cs.ListUserDataResponse); ok {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tPROJECT\tACCOUNT")
			for _, u := range resp.UserData {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.Name, u.Id, u.Project, u.Account)
			}
			w.Flush()
			return nil
		}
	}

	// Fallback: pretty-print JSON for unknown kinds or unexpected types
	b, _ := json.MarshalIndent(obj, "", "  ")
	fmt.Println(string(b))
	return nil
}

// PrintVolumes prints a table of volumes.
func PrintVolumes(vols []*cs.Volume) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tVM\tTYPE\tSTATUS")
	for _, v := range vols {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", v.Name, v.Id, v.Vmname, v.Type, v.State)
	}
	w.Flush()
}

// PrintNetworks prints a table of networks.
func PrintNetworks(nets []*cs.Network) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tZONE\tVLAN\tDISPLAY TEXT\tTYPE\tSTATE")
	client, _ := cloudstack.NewClient()

	for _, n := range nets {
		display := n.Displaytext
		if display == "" {
			display = n.Name
		}

		zoneName := n.Zoneid
		if n.Zoneid != "" && client != nil {
			zp := client.Zone.NewListZonesParams()
			zp.SetId(n.Zoneid)
			zr, zerr := client.Zone.ListZones(zp)
			if zerr == nil && zr != nil && len(zr.Zones) > 0 {
				zoneName = zr.Zones[0].Name
			}
		}

		vlan := ""
		if b, merr := json.Marshal(n); merr == nil {
			var m map[string]interface{}
			if uerr := json.Unmarshal(b, &m); uerr == nil {
				if v, ok := m["vlan"].(string); ok {
					vlan = v
				} else if v, ok := m["vlanid"].(string); ok {
					vlan = v
				}
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", n.Name, n.Id, zoneName, vlan, display, n.Type, n.State)
	}
	w.Flush()
}

// PrintVMsFromDB prints VMs returned by the controller DB query.
func PrintVMsFromDB(vms []v1.VirtualMachine) {
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
