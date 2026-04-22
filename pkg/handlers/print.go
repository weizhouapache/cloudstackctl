package handlers

import (
	"cloudstackctl/pkg/cloudstack"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	v1 "cloudstackctl/apis/v1"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
	"golang.org/x/term"
)

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiCyan    = "\033[36m"
	ansiBlue    = "\033[34m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiMagenta = "\033[35m"
	ansiRed     = "\033[31m"
)

// colorMode controls color output:
//
//	"auto"   – colors only when stdout is a TTY (default)
//	"always" – force colors on
//	"never"  – force colors off
var colorMode = "auto"

// SetColorMode sets the color output mode. Valid values: "auto", "always", "never".
func SetColorMode(mode string) {
	colorMode = mode
}

func shellColorEnabled() bool {
	switch colorMode {
	case "always":
		return true
	case "never":
		return false
	}
	// "auto": respect env vars and TTY detection
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// tabwriterEsc is the escape byte used by tabwriter to mark invisible sequences.
// Content wrapped in \xff pairs is excluded from column-width calculations.
const tabwriterEsc = "\xff"

func colorize(s, color string) string {
	if !shellColorEnabled() {
		return s
	}
	// Wrap ANSI codes in tabwriter escape markers so they are not counted
	// as visible characters when computing column widths.
	return tabwriterEsc + color + tabwriterEsc + s + tabwriterEsc + ansiReset + tabwriterEsc
}

func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.StripEscape)
}

func colorHeader(s string) string {
	return colorize(s, ansiBold+ansiBlue)
}

func colorResourceName(kind, s string) string {
	switch kind {
	case "Volume":
		return colorize(s, ansiCyan)
	case "Network":
		return colorize(s, ansiYellow)
	case "VirtualMachine":
		return colorize(s, ansiGreen)
	case "Template":
		return colorize(s, ansiMagenta)
	case "SSHKey":
		return colorize(s, ansiBlue)
	case "SecurityGroup":
		return colorize(s, ansiRed)
	case "AffinityGroup":
		return colorize(s, ansiMagenta)
	case "UserData":
		return colorize(s, ansiYellow)
	case "Project":
		return colorize(s, ansiCyan)
	case "Component":
		return colorize(s, ansiYellow)
	case "VMSpec":
		return colorize(s, ansiMagenta)
	case "Application":
		return colorize(s, ansiCyan)
	default:
		return s
	}
}

func colorStatus(s string) string {
	switch s {
	case "Running", "Healthy", "Implemented", "Setup", "Ready":
		return colorize(s, ansiGreen)
	case "Allocated", "Starting", "Started", "Creating", "Stopping":
		return colorize(s, ansiYellow)
	case "Error", "Failed", "Destroyed", "VMNotFound", "Unhealthy":
		return colorize(s, ansiRed)
	default:
		return s
	}
}

func colorBool(v bool) string {
	if v {
		return colorize("true", ansiGreen)
	}
	return colorize("false", ansiRed)
}

func colorDrift(v bool) string {
	if v {
		return colorize("true", ansiRed)
	}
	return colorize("false", ansiGreen)
}

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
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("VIRTUAL MACHINE")+"\t"+colorHeader("ID")+"\t"+colorHeader("IP ADDRESS")+"\t"+colorHeader("PUBLIC IP")+"\t"+colorHeader("SERVICE OFFERING")+"\t"+colorHeader("STATUS"))
			for _, v := range resp.VirtualMachines {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", colorResourceName("VirtualMachine", v.Name), v.Id, v.Ipaddress, v.Publicip, v.Serviceofferingname, colorStatus(v.State))
			}
			w.Flush()
			return nil
		}
	case "Template":
		if resp, ok := obj.(*cs.ListTemplatesResponse); ok {
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("TEMPLATE")+"\t"+colorHeader("ID")+"\t"+colorHeader("OS")+"\t"+colorHeader("FEATURED"))
			for _, t := range resp.Templates {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", colorResourceName("Template", t.Name), t.Id, t.Ostypename, colorBool(t.Isfeatured))
			}
			w.Flush()
			return nil
		}
	case "SSHKey":
		if resp, ok := obj.(*cs.ListSSHKeyPairsResponse); ok {
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("SSH KEY")+"\t"+colorHeader("FINGERPRINT"))
			for _, k := range resp.SSHKeyPairs {
				fmt.Fprintf(w, "%s\t%s\n", colorResourceName("SSHKey", k.Name), k.Fingerprint)
			}
			w.Flush()
			return nil
		}
	case "SecurityGroup":
		if resp, ok := obj.(*cs.ListSecurityGroupsResponse); ok {
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("SECURITY GROUP")+"\t"+colorHeader("ID")+"\t"+colorHeader("DESCRIPTION"))
			for _, sg := range resp.SecurityGroups {
				fmt.Fprintf(w, "%s\t%s\t%s\n", colorResourceName("SecurityGroup", sg.Name), sg.Id, sg.Description)
			}
			w.Flush()
			return nil
		}
	case "AffinityGroup":
		if resp, ok := obj.(*cs.ListAffinityGroupsResponse); ok {
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("AFFINITY GROUP")+"\t"+colorHeader("ID")+"\t"+colorHeader("DESCRIPTION"))
			for _, a := range resp.AffinityGroups {
				fmt.Fprintf(w, "%s\t%s\t%s\n", colorResourceName("AffinityGroup", a.Name), a.Id, a.Description)
			}
			w.Flush()
			return nil
		}
	case "UserData":
		if resp, ok := obj.(*cs.ListUserDataResponse); ok {
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("USERDATA")+"\t"+colorHeader("ID")+"\t"+colorHeader("ACCOUNT"))
			for _, u := range resp.UserData {
				fmt.Fprintf(w, "%s\t%s\t%s\n", colorResourceName("UserData", u.Name), u.Id, u.Account)
			}
			w.Flush()
			return nil
		}
	case "Project":
		if resp, ok := obj.(*cs.ListProjectsResponse); ok {
			w := newTabWriter()
			fmt.Fprintln(w, colorHeader("PROJECT")+"\t"+colorHeader("ID")+"\t"+colorHeader("STATE")+"\t"+colorHeader("DISPLAY TEXT"))
			for _, p := range resp.Projects {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", colorResourceName("Project", p.Name), p.Id, colorStatus(p.State), p.Displaytext)
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
	w := newTabWriter()
	fmt.Fprintln(w, colorHeader("VOLUME")+"\t"+colorHeader("ID")+"\t"+colorHeader("VIRTUAL MACHINE")+"\t"+colorHeader("TYPE")+"\t"+colorHeader("STATUS"))
	for _, v := range vols {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", colorResourceName("Volume", v.Name), v.Id, v.Vmname, v.Type, colorStatus(v.State))
	}
	w.Flush()
}

// PrintNetworks prints a table of networks.
func PrintNetworks(nets []*cs.Network) {
	w := newTabWriter()
	fmt.Fprintln(w, colorHeader("NETWORK")+"\t"+colorHeader("ID")+"\t"+colorHeader("VLAN")+"\t"+colorHeader("DISPLAY TEXT")+"\t"+colorHeader("TYPE")+"\t"+colorHeader("STATE"))

	for _, n := range nets {
		display := n.Displaytext
		if display == "" {
			display = n.Name
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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", colorResourceName("Network", n.Name), n.Id, vlan, display, n.Type, colorStatus(n.State))
	}
	w.Flush()
}

// PrintVMsFromController prints VMs returned by the controller query.
func PrintVMsFromController(vms []v1.VirtualMachine) {
	w := newTabWriter()
	fmt.Fprintln(w, colorHeader("VIRTUAL MACHINE")+"\t"+colorHeader("APPLICATION")+"\t"+colorHeader("COMPONENT")+"\t"+colorHeader("ID")+"\t"+colorHeader("IP ADDRESS")+"\t"+colorHeader("PUBLIC IP")+"\t"+colorHeader("SERVICE OFFERING")+"\t"+colorHeader("STATUS")+"\t"+colorHeader("READY")+"\t"+colorHeader("DRIFT"))
	client, _ := cloudstack.NewClient()
	for _, vm := range vms {
		id := vm.CloudStackID
		ipAddress := ""
		publicIP := ""
		if client != nil {
			params := client.VirtualMachine.NewListVirtualMachinesParams()
			if id != "" {
				params.SetId(id)
			} else {
				params.SetName(vm.Metadata.Name)
			}
			if resp, err := client.VirtualMachine.ListVirtualMachines(params); err == nil && resp != nil && len(resp.VirtualMachines) > 0 {
				v := resp.VirtualMachines[0]
				for _, nic := range v.Nic {
					if nic.Ipaddress != "" {
						ipAddress = nic.Ipaddress
						break
					}
				}
				if v.Publicip != "" {
					publicIP = v.Publicip
				}
			}
		}
		so := vm.Spec.ServiceOffering
		if so == "" && vm.ObservedSpec.ServiceOffering != "" {
			so = vm.ObservedSpec.ServiceOffering
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			colorResourceName("VirtualMachine", vm.Metadata.Name),
			vm.Application,
			vm.Component,
			id,
			ipAddress,
			publicIP,
			so,
			colorStatus(vm.Status.ObservedState),
			colorBool(vm.Status.Ready),
			colorDrift(vm.Status.Drift),
		)
	}
	w.Flush()
}

// PrintComponents prints components returned by the controller DB query.
func PrintComponents(comps []v1.Component) {
	w := newTabWriter()
	fmt.Fprintln(w, colorHeader("COMPONENT")+"\t"+colorHeader("APPLICATION")+"\t"+colorHeader("REPLICAS")+"\t"+colorHeader("VM SPEC")+"\t"+colorHeader("STATE")+"\t"+colorHeader("OBSERVED REPLICAS")+"\t"+colorHeader("LAST CHECKED")+"\t"+colorHeader("CREATED"))

	for _, c := range comps {
		replicas := c.Spec.Replicas
		vmSpec := c.Spec.VirtualMachineSpec
		observed := c.ObservedReplicas
		state := c.Status.ObservedState
		appNames := c.Application
		last := ""
		if !c.Status.LastChecked.IsZero() {
			last = c.Status.LastChecked.Format(time.RFC3339)
		}
		created := ""
		if !c.CreatedAt.IsZero() {
			created = c.CreatedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%d\t%s\t%s\n", colorResourceName("Component", c.Metadata.Name), appNames, replicas, vmSpec, colorStatus(state), observed, last, created)
	}
	w.Flush()
}

// PrintVMSpecs prints VirtualMachineSpecResource entries in a compact table.
func PrintVMSpecs(specs []v1.VirtualMachineSpecResource) {
	w := newTabWriter()
	fmt.Fprintln(w, colorHeader("VM SPEC")+"\t"+colorHeader("TEMPLATE")+"\t"+colorHeader("SERVICE OFFERING")+"\t"+colorHeader("NETWORKS")+"\t"+colorHeader("VOLUMES")+"\t"+colorHeader("CREATED"))
	for _, s := range specs {
		tmpl := s.Spec.Template
		so := s.Spec.ServiceOffering
		nets := ""
		if len(s.Spec.Networks) > 0 {
			nets = s.Spec.Networks[0]
			for i := 1; i < len(s.Spec.Networks); i++ {
				nets += "," + s.Spec.Networks[i]
			}
		}
		volCount := 1
		if len(s.Spec.Volumes) > 0 {
			for _, vs := range s.Spec.Volumes {
				if vs.Type == "data" {
					volCount++
				}
			}
		}
		created := ""
		if !s.CreatedAt.IsZero() {
			created = s.CreatedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n", colorResourceName("VMSpec", s.Metadata.Name), tmpl, so, nets, volCount, created)
	}
	w.Flush()
}

// PrintApplications prints applications returned by the controller DB query.
func PrintApplications(apps []v1.Application) {
	w := newTabWriter()
	fmt.Fprintln(w, colorHeader("APPLICATION")+"\t"+colorHeader("COMPONENTS")+"\t"+colorHeader("STATE")+"\t"+colorHeader("READY")+"\t"+colorHeader("LAST CHECKED")+"\t"+colorHeader("CREATED"))
	for _, a := range apps {
		compNames := ""
		if len(a.Spec.Components) > 0 {
			compNames = a.Spec.Components[0].Name
			for i := 1; i < len(a.Spec.Components); i++ {
				compNames += "," + a.Spec.Components[i].Name
			}
		}
		last := ""
		if !a.Status.LastChecked.IsZero() {
			last = a.Status.LastChecked.Format(time.RFC3339)
		}
		created := ""
		if !a.CreatedAt.IsZero() {
			created = a.CreatedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", colorResourceName("Application", a.Metadata.Name), compNames, colorStatus(a.Status.ObservedState), colorBool(a.Status.Ready), last, created)
	}
	w.Flush()
}
