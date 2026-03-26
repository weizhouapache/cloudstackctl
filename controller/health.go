package controller

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
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

	// Get all VMs in component by matching the `component` column which stores
	// the owning component name. This is more reliable than depending on
	// labels JSON content.
	var vms []v1.VirtualMachine
	if err := db.DB.Where("component = ?", component.Metadata.Name).Find(&vms).Error; err != nil {
		return false, err
	}

	// Check each VM health. Skip VMs that are already marked Removing.
	healthyCount := 0
	for _, vm := range vms {
		if vm.Status.ObservedState == "Removing" {
			log.Printf("Skipping health check for VM %s: state=Removing", vm.Metadata.Name)
			continue
		}
		healthy, err := c.CheckVMHealth(&vm)
		if err != nil {
			log.Printf("VM %s health check failed: %v", vm.Metadata.Name, err)
			continue
		}
		if healthy {
			healthyCount++
		}
	}

	// Determine required minimum healthy VMs (default to replicas)
	req := component.Spec.MinHealthy
	if req <= 0 {
		req = component.Spec.Replicas
	}

	isHealthy := healthyCount >= req

	// Update component status, but do not overwrite if component is marked Removing.
	if component.Status.ObservedState != "Removing" {
		component.Status.Ready = isHealthy
		component.Status.LastChecked = time.Now()
		if isHealthy {
			component.Status.ObservedState = "Healthy"
		} else {
			component.Status.ObservedState = "Unhealthy"
		}
		// log the health status for visibility
		log.Printf("Component %s health check: %d/%d healthy (required: %d)", component.Metadata.Name, healthyCount, len(vms), req)
		return isHealthy, db.DB.Save(component).Error
	}
	// If component is Removing, only update LastChecked timestamp
	component.Status.LastChecked = time.Now()
	return isHealthy, db.DB.Save(component).Error
}

// CheckVMHealth performs ping/SSH health check for a VM
func (c *Controller) CheckVMHealth(vm *v1.VirtualMachine) (bool, error) {
	// If VM is marked Removing, do not update state during health checks.
	if vm.Status.ObservedState == "Removing" {
		return false, nil
	}

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
		vm.Status.ObservedState = "VMNotFound"
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

	// Determine health checks to run: use Spec.HealthChecks if present, otherwise return healthy (no checks to run)
	checks := vm.Spec.HealthChecks
	// If no checks defined, fall back to component health checks if VM is part of a component
	if len(checks) == 0 && vm.Component != "" {
		var comp v1.Component
		if err := db.DB.Where("name = ?", vm.Component).First(&comp).Error; err == nil {
			checks = comp.Spec.HealthChecks
		}
	}

	// If still no checks defined, consider the VM healthy if it has an IP and is running in CloudStack.
	if len(checks) == 0 {
		log.Printf("Health check passed for VM %s (id=%s): no health checks defined, defaulting to healthy", vm.Metadata.Name, vm.CloudStackID)
		vm.Status.ObservedState = "Healthy"
		vm.Status.Ready = true
		vm.Status.LastChecked = time.Now()
		return true, db.DB.Save(vm).Error
	}

	if vmIP == "" {
		log.Printf("no IP address found for VM %s (id=%s)", vm.Metadata.Name, vm.CloudStackID)
		vm.Status.ObservedState = "IPNotFound"
		vm.Status.Ready = false
		vm.Status.LastChecked = time.Now()
		return false, db.DB.Save(vm).Error
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
		case "tcp":
			// Generic TCP connect check; default port 80 when not specified.
			port := "80"
			if hc.Port != 0 {
				port = fmt.Sprintf("%d", hc.Port)
			}
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(vmIP, port), timeout)
			if err != nil {
				log.Printf("VM %s TCP check to %s:%s failed: %v", vm.Metadata.Name, vmIP, port, err)
				overallHealthy = false
			} else {
				conn.Close()
			}
		case "http", "https":
			// HTTP/HTTPS GET check. Default port 80 for http and 443 for https.
			scheme := "http"
			if hc.Type == "https" {
				scheme = "https"
			}
			port := ""
			if hc.Port != 0 {
				port = fmt.Sprintf("%d", hc.Port)
			} else {
				if scheme == "http" {
					port = "80"
				} else {
					port = "443"
				}
			}
			path := "/"
			if hc.Path != "" {
				path = hc.Path
				if !strings.HasPrefix(path, "/") {
					path = "/" + path
				}
			}
			// Use net.JoinHostPort to properly bracket IPv6 addresses
			hostPort := net.JoinHostPort(vmIP, port)
			urlStr := fmt.Sprintf("%s://%s%s", scheme, hostPort, path)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
			if err != nil {
				log.Printf("VM %s %s check to %s failed to build request: %v", vm.Metadata.Name, hc.Type, urlStr, err)
				overallHealthy = false
				break
			}
			client := &http.Client{Timeout: timeout}
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("VM %s %s check to %s failed: %v", vm.Metadata.Name, hc.Type, urlStr, err)
				overallHealthy = false
			} else {
				resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 400 {
					log.Printf("VM %s %s check to %s returned status %d", vm.Metadata.Name, hc.Type, urlStr, resp.StatusCode)
					overallHealthy = false
				}
			}
		default:
			// Unknown check type: mark as not healthy and log
			log.Printf("Unknown health check type %s for VM %s", hc.Type, vm.Metadata.Name)
			overallHealthy = false
		}
	}

	// Only update ObservedState if VM is not marked Removing.
	if vm.Status.ObservedState != "Removing" {
		vm.Status.Ready = overallHealthy
		vm.Status.LastChecked = time.Now()
		if overallHealthy {
			vm.Status.ObservedState = "Healthy"
		} else {
			vm.Status.ObservedState = "Unhealthy"
		}
		return overallHealthy, db.DB.Save(vm).Error
	}
	// If Removing, only update LastChecked timestamp
	vm.Status.LastChecked = time.Now()
	return overallHealthy, db.DB.Save(vm).Error
}
