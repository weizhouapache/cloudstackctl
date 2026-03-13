package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/cloudstack"
	"log"
	"time"
)

// DetectDrift checks if VM desired state != observed state in CloudStack
func (c *Controller) DetectDrift(vm *v1.VirtualMachine) error {
	log.Printf("Checking drift for VM: %s", vm.Metadata.Name)

	// Skip if VM not created in CloudStack
	if vm.Status.CloudStackID == "" {
		vm.Status.Drift = false
		return nil
	}

	// Get actual VM state from CloudStack
	actualState, err := cloudstack.GetVMState(c.csClient, vm.Status.CloudStackID)
	if err != nil {
		return err
	}

	// Check for configuration drift
	driftDetected := false

	// 1. Check if VM state matches desired state
	if actualState != "Running" && vm.Status.ObservedState == "Running" {
		driftDetected = true
	}

	// 2. Check if VM configuration matches spec (template/service offering)
	// (Extend with more checks as needed)

	// Update drift status
	vm.Status.Drift = driftDetected
	vm.Status.LastChecked = time.Now()

	if driftDetected {
		log.Printf("Drift detected for VM %s: desired=%s, actual=%s", vm.Metadata.Name, vm.Status.ObservedState, actualState)
		// Auto-heal: recreate VM if drift detected (optional)
		// return c.recreateVM(vm)
	}

	return db.DB.Save(vm).Error
}

// ReconcileDrift fixes detected drift by updating CloudStack resources
func (c *Controller) ReconcileDrift(vm *v1.VirtualMachine) error {
	if !vm.Status.Drift {
		return nil
	}

	log.Printf("Reconciling drift for VM: %s", vm.Metadata.Name)

	// Example: Stop and restart VM with correct configuration
	// (Implement based on drift type)
	return nil
}
