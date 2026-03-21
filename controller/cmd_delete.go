package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/cloudstack"
	"log"
)

// DeleteApplication deletes an Application and disassociates its Components
// (it does NOT delete VMs; VMs are owned by the Application lifecycle)
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
		// Remove the component record but do NOT delete VMs here; VMs belong to the application lifecycle.
		db.DB.Where("name = ?", compRef.Name).Delete(&v1.Component{})
		log.Printf("Component %s disassociated from application %s", compRef.Name, name)
	}

	if err := db.DB.Delete(&app).Error; err != nil {
		log.Fatalf("Failed to delete application %s: %v", name, err)
	}

	log.Printf("Application %s deleted successfully", name)
}

// DeleteComponent deletes a Component record if it's not referenced by any
// Application. It does NOT delete VMs (VMs belong to applications).
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

	// Do not delete VMs here. Prevent deletion if the component is still referenced by any Application.
	var apps []v1.Application
	if err := db.DB.Find(&apps).Error; err == nil {
		for _, a := range apps {
			for _, cref := range a.Spec.Components {
				if cref.Name == name {
					log.Fatalf("Component %s is still referenced by Application %s; cannot delete", name, a.Metadata.Name)
				}
			}
		}
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
