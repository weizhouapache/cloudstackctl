package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
)

// ListApplications prints applications or a single application when name provided.
func ListApplications(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Printf("Database unavailable: %v", err)
			return
		}
	}

	if name != "" {
		var app v1.Application
		if err := db.DB.Where("name = ?", name).First(&app).Error; err != nil {
			log.Fatalf("Application %s not found: %v", name, err)
		}
		b, _ := json.MarshalIndent(app, "", "  ")
		fmt.Println(string(b))
		return
	}

	var apps []v1.Application
	if err := db.DB.Find(&apps).Error; err != nil {
		log.Fatalf("Failed to list applications: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROJECT\tSTATUS\tREADY\tDRIFT")

	for _, app := range apps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%t\n",
			app.Metadata.Name,
			app.Spec.Project,
			app.Status.ObservedState,
			app.Status.Ready,
			app.Status.Drift,
		)
	}

	w.Flush()
}

// ListComponents prints components or a single component when name provided.
func ListComponents(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Printf("Database unavailable: %v", err)
			return
		}
	}

	if name != "" {
		var comp v1.Component
		if err := db.DB.Where("name = ?", name).First(&comp).Error; err != nil {
			log.Fatalf("Component %s not found: %v", name, err)
		}
		b, _ := json.MarshalIndent(comp, "", "  ")
		fmt.Println(string(b))
		return
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

// ListVirtualMachineSpec prints VM specs or a single spec when name provided.
func ListVirtualMachineSpec(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}

	if name != "" {
		var vs v1.VirtualMachineSpecResource
		if err := db.DB.Where("name = ?", name).First(&vs).Error; err != nil {
			log.Fatalf("VirtualMachineSpec %s not found: %v", name, err)
		}
		b, _ := json.MarshalIndent(vs, "", "  ")
		fmt.Println(string(b))
		return
	}

	var specs []v1.VirtualMachineSpecResource
	if err := db.DB.Find(&specs).Error; err != nil {
		log.Fatalf("Failed to list VirtualMachineSpec: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTEMPLATE\tSERVICE_OFFERING\tNETWORKS\tVOLUMES")
	for _, s := range specs {
		nets := strings.Join(s.Spec.NetworkIDs, ",")
		volCount := 0
		if len(s.Spec.Volumes) > 0 {
			volCount = len(s.Spec.Volumes)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n",
			s.Metadata.Name,
			s.Spec.Template,
			s.Spec.ServiceOffering,
			nets,
			volCount,
		)
	}
	w.Flush()
}

// ListVMs prints VMs from DB; supports single VM by name.
func ListVMs(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Printf("Database unavailable: %v", err)
			return
		}
	}

	if name != "" {
		var vm v1.VirtualMachine
		if err := db.DB.Where("name = ?", name).First(&vm).Error; err != nil {
			log.Fatalf("VirtualMachine %s not found: %v", name, err)
		}
		b, _ := json.MarshalIndent(vm, "", "  ")
		fmt.Println(string(b))
		return
	}

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
