package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/cloudstack"
	"log"
)

// DeleteApplication deletes an Application and its dependent resources
func DeleteApplication(name string) {
	var app v1.Application
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}
	if err := db.DB.Where("name = ?", name).First(&app).Error; err != nil {
		log.Fatalf("Application %s not found: %v", name, err)
	}

	for _, compRef := range app.Spec.Components {
		DeleteComponent(compRef.Name)
	}

	if err := db.DB.Delete(&app).Error; err != nil {
		log.Fatalf("Failed to delete application %s: %v", name, err)
	}

	log.Printf("Application %s deleted successfully", name)
}

// DeleteComponent deletes a Component and its VMs
func DeleteComponent(name string) {
	var comp v1.Component
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}
	if err := db.DB.Where("name = ?", name).First(&comp).Error; err != nil {
		log.Fatalf("Component %s not found: %v", name, err)
	}

	var vms []v1.VirtualMachine
	if err := db.DB.Where("metadata_labels @> ?", map[string]string{"component": name}).Find(&vms).Error; err != nil {
		log.Fatalf("Failed to find VMs for component %s: %v", name, err)
	}

	for _, vm := range vms {
		DeleteVM(vm.Metadata.Name)
	}

	if err := db.DB.Delete(&comp).Error; err != nil {
		log.Fatalf("Failed to delete component %s: %v", name, err)
	}

	log.Printf("Component %s deleted successfully", name)
}

// DeleteVM deletes a VM from CloudStack and database
func DeleteVM(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}

	var vm v1.VirtualMachine
	if err := db.DB.Where("name = ?", name).First(&vm).Error; err != nil {
		// Fallback: try CloudStack delete by name
		csClient, cerr := cloudstack.NewClient()
		if cerr != nil {
			log.Fatalf("VM %s not found and CloudStack client unavailable: %v", name, cerr)
		}
		params := csClient.VirtualMachine.NewListVirtualMachinesParams()
		params.SetName(name)
		resp, _ := csClient.VirtualMachine.ListVirtualMachines(params)
		if resp != nil && len(resp.VirtualMachines) > 0 {
			id := resp.VirtualMachines[0].Id
			dp := csClient.VirtualMachine.NewDestroyVirtualMachineParams(id)
			dp.SetExpunge(true)
			if _, err := csClient.VirtualMachine.DestroyVirtualMachine(dp); err != nil {
				log.Printf("Warning: Failed to delete VM %s from CloudStack: %v", name, err)
			}
			log.Printf("VM %s deleted from CloudStack (id=%s)", name, id)
			return
		}
		log.Fatalf("VM %s not found: %v", name, err)
	}

	if vm.Status.CloudStackID != "" {
		csClient, err := cloudstack.NewClient()
		if err != nil {
			log.Printf("Warning: CloudStack client unavailable, skipping CloudStack delete: %v", err)
		} else {
			params := csClient.VirtualMachine.NewDestroyVirtualMachineParams(vm.Status.CloudStackID)
			if _, err := csClient.VirtualMachine.DestroyVirtualMachine(params); err != nil {
				log.Printf("Warning: Failed to delete VM %s from CloudStack: %v", name, err)
			}
		}
	}

	if err := db.DB.Delete(&vm).Error; err != nil {
		log.Fatalf("Failed to delete VM %s from database: %v", name, err)
	}

	log.Printf("VM %s deleted successfully", name)
}
