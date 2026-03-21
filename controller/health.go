package controller

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"log"
	"net"
)

// fmtTimeoutSeconds converts a time.Duration to a seconds string for ping -W
func fmtTimeoutSeconds(d time.Duration) string {
	s := int(d.Seconds())
	if s <= 0 {
		s = 1
	}
	return fmt.Sprintf("%d", s)
}

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
	if vm.CloudStackID == "" {
		return false, nil
	}
	// Query CloudStack for the VM to obtain its IP(s)
	params := c.csClient.VirtualMachine.NewListVirtualMachinesParams()
	params.SetId(vm.CloudStackID)
	resp, err := c.csClient.VirtualMachine.ListVirtualMachines(params)
	if err != nil {
		log.Printf("failed to describe VM %s: %v", vm.Metadata.Name, err)
		vm.Status.Ready = false
		vm.Status.LastChecked = time.Now()
		return false, db.DB.Save(vm).Error
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		log.Printf("no CloudStack VM found for %s (id=%s)", vm.Metadata.Name, vm.CloudStackID)
		vm.Status.Ready = false
		vm.Status.LastChecked = time.Now()
		return false, db.DB.Save(vm).Error
	}

	v := resp.VirtualMachines[0]

	// extract an IP address from NICs (prefer IPv4)
	vmIP := ""
	for _, n := range v.Nic {
		if n.Ipaddress != "" {
			vmIP = n.Ipaddress
			break
		}
	}
	if vmIP == "" {
		log.Printf("no IP address found for VM %s (id=%s)", vm.Metadata.Name, vm.CloudStackID)
		vm.Status.Ready = false
		vm.Status.LastChecked = time.Now()
		return false, db.DB.Save(vm).Error
	}

	// Determine health checks to run: use Spec.HealthChecks if present, otherwise return healthy (no checks to run)
	checks := vm.Spec.HealthChecks
	if len(checks) == 0 {
		return true, nil
	}

	overallHealthy := true
	for _, hc := range checks {
		timeout := 5 * time.Second
		if hc.Timeout != "" {
			if d, err := time.ParseDuration(hc.Timeout); err == nil {
				timeout = d
			}
		}

		switch hc.Type {
		case "ping":
			// Use system ping command (platform: Linux). Run with context timeout.
			ctx, cancel := context.WithTimeout(context.Background(), timeout+1*time.Second)
			defer cancel()
			// `-c 1` send one packet, `-W` sets timeout in seconds for Linux ping
			cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", fmtTimeoutSeconds(timeout), vmIP)
			if err := cmd.Run(); err != nil {
				log.Printf("VM %s ping check to %s failed: %v", vm.Metadata.Name, vmIP, err)
				overallHealthy = false
			}
		case "ssh":
			// TCP connect to SSH port (default 22)
			port := "22"
			if hc.Port != 0 {
				port = fmt.Sprintf("%d", hc.Port)
			}
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(vmIP, port), timeout)
			if err != nil {
				log.Printf("VM %s SSH check to %s:%s failed: %v", vm.Metadata.Name, vmIP, port, err)
				overallHealthy = false
			} else {
				conn.Close()
			}
		default:
			// Unknown check type: mark as not healthy and log
			log.Printf("Unknown health check type %s for VM %s", hc.Type, vm.Metadata.Name)
			overallHealthy = false
		}
	}

	vm.Status.Ready = overallHealthy
	vm.Status.LastChecked = time.Now()
	if overallHealthy {
		vm.Status.ObservedState = "Healthy"
	} else {
		vm.Status.ObservedState = "Unhealthy"
	}

	return overallHealthy, db.DB.Save(vm).Error
}
