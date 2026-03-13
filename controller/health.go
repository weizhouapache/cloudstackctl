package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"log"
	"net"
	"time"
)

// CheckComponentHealth verifies all VMs in a component are healthy
func (c *Controller) CheckComponentHealth(component *v1.Component) (bool, error) {
	log.Printf("Checking health for component: %s", component.Metadata.Name)

	// Get all VMs in component
	var vms []v1.VirtualMachine
	if err := db.DB.Where("metadata_labels @> ?", map[string]string{"component": component.Metadata.Name}).Find(&vms).Error; err != nil {
		return false, err
	}

	// Check each VM health
	allHealthy := true
	for _, vm := range vms {
		healthy, err := c.CheckVMHealth(&vm)
		if err != nil {
			log.Printf("VM %s health check failed: %v", vm.Metadata.Name, err)
			allHealthy = false
			continue
		}

		if !healthy {
			allHealthy = false
		}
	}

	// Update component status
	component.Status.Ready = allHealthy
	component.Status.LastChecked = time.Now()
	if allHealthy {
		component.Status.ObservedState = "Healthy"
	} else {
		component.Status.ObservedState = "Unhealthy"
	}

	return allHealthy, db.DB.Save(component).Error
}

// CheckVMHealth performs ping/SSH health check for a VM
func (c *Controller) CheckVMHealth(vm *v1.VirtualMachine) (bool, error) {
	// Skip if VM not created in CloudStack
	if vm.Status.CloudStackID == "" {
		return false, nil
	}

	// Get VM IP from CloudStack (implement with SDK)
	vmIP := vm.Status.ObservedState // Replace with actual IP retrieval

	// 1. TCP ping to port 22 (SSH)
	conn, err := net.DialTimeout("tcp", vmIP+":22", 5*time.Second)
	if err != nil {
		log.Printf("VM %s SSH check failed: %v", vm.Metadata.Name, err)
		vm.Status.Ready = false
		return false, db.DB.Save(vm).Error
	}
	defer conn.Close()

	// 2. Additional health checks (HTTP/ping/custom) can be added here

	// Update VM status
	vm.Status.Ready = true
	vm.Status.LastChecked = time.Now()
	return true, db.DB.Save(vm).Error
}
