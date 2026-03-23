package controller

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/handlers"
)

// ReconcileAll runs reconciliation for all resources
func (c *Controller) ReconcileAll() {
	log.Println("Starting reconciliation loop")

	// Manage per-application workers: start a worker for each non-removing
	// application and stop workers for applications that are removed.
	var apps []v1.Application
	if err := db.DB.Find(&apps).Error; err != nil {
		log.Printf("Failed to list applications: %v", err)
		return
	}
	for _, app := range apps {
		if app.Status.ObservedState == "Removing" {
			// ensure worker stopped for removing apps
			c.stopAppWorker(app.Metadata.Name)
			continue
		}
		// start a worker if not present
		c.startAppWorker(app.Metadata.Name)
	}
	// Reconcile VMs that are not part of any application or component.
	var vms []v1.VirtualMachine
	if err := db.DB.Where("deleted_at IS NULL AND observed_state <> ? AND (application IS NULL OR application = '') AND (component IS NULL OR component = '')", "Removing").Find(&vms).Error; err != nil {
		log.Printf("Failed to list VMs: %v", err)
		return
	}

	for _, vm := range vms {
		if err := c.ReconcileVM(&vm); err != nil {
			log.Printf("Failed to reconcile VM %s: %v", vm.Metadata.Name, err)
		}
	}

	// Remove Removing components that are not part of any application.
	var comps []v1.Component
	if err := db.DB.Where("deleted_at IS NULL AND observed_state = ? AND (application IS NULL OR application = '')", "Removing").Find(&comps).Error; err != nil {
		log.Printf("Failed to list components: %v", err)
		return
	}
	for _, comp := range comps {
		log.Printf("Deleting component %s marked as Removing and not linked to any application", comp.Metadata.Name)
		if err := db.DB.Delete(&comp).Error; err != nil {
			log.Printf("Failed to delete component %s: %v", comp.Metadata.Name, err)
		}
	}
}

// ReconcileApplication ensures application state matches desired state
func (c *Controller) ReconcileApplication(app *v1.Application) error {
	// Skip applications that are marked for removal
	if app.Status.ObservedState == "Removing" {
		return nil
	}

	// Normal reconciliation for this application: VMs -> Components -> Application
	// Reconcile VMs belonging to this application
	var vms []v1.VirtualMachine
	if err := db.DB.Where("application = ? AND (observed_state IS NULL OR observed_state <> ?)", app.Metadata.Name, "Removing").Find(&vms).Error; err == nil {
		for _, vm := range vms {
			if err := c.ReconcileVM(&vm); err != nil {
				log.Printf("app worker: failed to reconcile VM %s: %v", vm.Metadata.Name, err)
			}
		}
	}

	// Reconcile components belonging to this application
	var comps []v1.Component
	if err := db.DB.Where("application = ? AND (observed_state IS NULL OR observed_state <> ?)", app.Metadata.Name, "Removing").Find(&comps).Error; err == nil {
		for _, comp := range comps {
			if err := c.ReconcileComponent(&comp); err != nil {
				log.Printf("app worker: failed to reconcile component %s: %v", comp.Metadata.Name, err)
			}
		}
	}

	// If all components are Healthy, mark application as Ready=true and ObservedState=Healthy. Otherwise, mark as Unhealthy.
	components := []v1.Component{}
	if err := db.DB.Where("application = ?", app.Metadata.Name).Find(&components).Error; err != nil {
		return err
	}

	ready := true
	for _, c := range components {
		if c.Status.ObservedState != "Healthy" {
			ready = false
			break
		}
	}

	app.Status.Ready = ready
	if ready {
		app.Status.ObservedState = "Healthy"
	} else {
		app.Status.ObservedState = "Unhealthy"
	}
	app.Status.LastChecked = time.Now()
	db.DB.Save(app)

	return nil
}

func (c *Controller) ReconcileRemovingApplication(app *v1.Application) error {
	name := app.Metadata.Name

	// 1) Destroy VMs belonging to this application that are marked Removing
	var removingVMs []v1.VirtualMachine
	if err := db.DB.Where("application = ? AND observed_state = ?", name, "Removing").Find(&removingVMs).Error; err == nil {
		for _, vm := range removingVMs {
			log.Printf("app worker: destroying VM %s", vm.Metadata.Name)
			if vm.CloudStackID != "" {
				dp := c.csClient.VirtualMachine.NewDestroyVirtualMachineParams(vm.CloudStackID)
				dp.SetExpunge(true)
				if _, err := c.csClient.VirtualMachine.DestroyVirtualMachine(dp); err != nil {
					log.Printf("app worker: failed to destroy VM %s (id=%s): %v", vm.Metadata.Name, vm.CloudStackID, err)
					return fmt.Errorf("failed to destroy VM %s: %w", vm.Metadata.Name, err)
				}
			}
			if err := db.DB.Delete(&vm).Error; err != nil {
				log.Printf("app worker: failed to delete VM record %s: %v", vm.Metadata.Name, err)
				return fmt.Errorf("failed to delete VM record %s: %w", vm.Metadata.Name, err)
			}
		}
	}

	// 2) Delete components belonging to this application that are marked Removing (only if no VMs reference them)
	var removingComps []v1.Component
	if err := db.DB.Where("application = ? AND observed_state = ?", name, "Removing").Find(&removingComps).Error; err == nil {
		for _, comp := range removingComps {
			var vmCount int64
			if err := db.DB.Model(&v1.VirtualMachine{}).Where("component = ?", comp.Metadata.Name).Count(&vmCount).Error; err != nil {
				log.Printf("app worker: failed to count VMs for component %s: %v", comp.Metadata.Name, err)
				return fmt.Errorf("failed to count VMs for component %s: %w", comp.Metadata.Name, err)
			}
			if vmCount > 0 {
				log.Printf("app worker: skipping deletion of component %s: %d VMs still exist", comp.Metadata.Name, vmCount)
				continue
			}
			log.Printf("app worker: removing component %s", comp.Metadata.Name)
			if err := db.DB.Delete(&comp).Error; err != nil {
				log.Printf("app worker: failed to delete component %s: %v", comp.Metadata.Name, err)
				return fmt.Errorf("failed to delete component %s: %w", comp.Metadata.Name, err)
			}
		}
	}

	// 3) If no components remain for the application, delete the application record and stop worker
	var remaining int64
	if err := db.DB.Model(&v1.Component{}).Where("application = ?", name).Count(&remaining).Error; err != nil {
		return fmt.Errorf("failed to count remaining components for application %s: %w", name, err)
	}
	if remaining == 0 {
		if err := db.DB.Delete(&app).Error; err != nil {
			log.Printf("app worker: failed to delete application %s: %v", name, err)
			return fmt.Errorf("failed to delete application %s: %w", name, err)
		} else {
			log.Printf("app worker: application %s removed", name)
		}
		// stop the worker after cleanup
		c.stopAppWorker(name)
		return nil
	}
	// If we reach here, it means there are still components or VMs that need to be cleaned up. The worker will continue running and pick them up in the next reconciliation loop.
	return fmt.Errorf("cleanup in progress, waiting for next reconciliation loop")
}

// ReconcileComponent ensures component state matches desired state
func (c *Controller) ReconcileComponent(comp *v1.Component) error {
	// Skip components that are marked for removal
	if comp.Status.ObservedState == "Removing" {
		return nil
	}

	// Check component health
	healthy, err := c.CheckComponentHealth(comp)
	if err != nil {
		return err
	}

	if !healthy {
		// Recreate unhealthy VMs
		return c.recreateComponentVMs(comp.Application, comp)
	}

	// Update observed replica count and persist effective spec if present
	var count int64
	if err := db.DB.Model(&v1.VirtualMachine{}).Where("name LIKE ?", comp.Metadata.Name+"-%").Count(&count).Error; err == nil {
		comp.ObservedReplicas = int(count)
	}

	// Persist component updates (observed replica count and any effective spec)
	if err := db.DB.Save(comp).Error; err != nil {
		return err
	}

	return nil
}

// ReconcileVM ensures VM state matches desired state
func (c *Controller) ReconcileVM(vm *v1.VirtualMachine) error {
	// Skip VMs marked for removal
	if vm.Status.ObservedState == "Removing" {
		return nil
	}
	// Populate observed spec from CloudStack (if possible)
	if err := c.populateObservedSpec(vm); err != nil {
		return err
	}

	// Check if VM exists; if not, create it
	if vm.CloudStackID == "" {
		if id, err := handlers.ApplyVirtualMachineManaged(vm, true); err != nil {
			// Remove VM if creation failed to avoid repeated creation attempts; user can recreate after fixing the issue.
			db.DB.Delete(vm)
			return err
		} else {
			if id != "" {
				vm.CloudStackID = id
				db.DB.Save(vm)
			}
		}
	}

	// Check for drift
	if err := c.DetectDrift(vm); err != nil {
		return err
	}

	// Reconcile drift if detected
	if vm.Status.Drift {
		return c.ReconcileDrift(vm)
	}

	// Check VM health
	_, err := c.CheckVMHealth(vm)
	return err
}

// populateObservedSpec queries CloudStack for VM details and fills ObservedSpec
func (c *Controller) populateObservedSpec(vm *v1.VirtualMachine) error {
	// Use SDK to list by id or name
	lp := c.csClient.VirtualMachine.NewListVirtualMachinesParams()
	if vm.CloudStackID != "" {
		lp.SetId(vm.CloudStackID)
	} else {
		lp.SetName(vm.Metadata.Name)
	}

	resp, err := c.csClient.VirtualMachine.ListVirtualMachines(lp)
	if err != nil {
		return err
	}
	if resp == nil || len(resp.VirtualMachines) == 0 {
		return nil
	}

	v := resp.VirtualMachines[0]

	// if vm.CloudStackID is not set, set it from the observed VM
	if vm.CloudStackID == "" && v.Id != "" {
		vm.CloudStackID = v.Id
	}

	// Map some observed fields into ObservedSpec using SDK types directly
	obs := vm.ObservedSpec
	if v.Templateid != "" {
		obs.Template = v.Templateid
	}
	if v.Serviceofferingid != "" {
		obs.ServiceOffering = v.Serviceofferingid
	}

	// NICs -> Networks
	if len(v.Nic) > 0 {
		obs.Networks = []string{}
		for _, n := range v.Nic {
			if n.Networkid != "" {
				obs.Networks = append(obs.Networks, n.Networkid)
			}
		}
	}

	// Keypairs (comma-separated string) -> SSHKeys
	if v.Keypairs != "" {
		// SDK returns comma-separated key names
		parts := strings.Split(v.Keypairs, ",")
		obs.SSHKeys = []string{}
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				obs.SSHKeys = append(obs.SSHKeys, s)
			}
		}
	}

	// Security groups
	if len(v.Securitygroup) > 0 {
		obs.SecurityGroups = []string{}
		for _, sg := range v.Securitygroup {
			if sg.Name != "" {
				obs.SecurityGroups = append(obs.SecurityGroups, sg.Name)
			} else if sg.Id != "" {
				obs.SecurityGroups = append(obs.SecurityGroups, sg.Id)
			}
		}
	}

	// Affinity groups
	if len(v.Affinitygroup) > 0 {
		obs.AffinityGroups = []string{}
		for _, ag := range v.Affinitygroup {
			if ag.Name != "" {
				obs.AffinityGroups = append(obs.AffinityGroups, ag.Name)
			} else if ag.Id != "" {
				obs.AffinityGroups = append(obs.AffinityGroups, ag.Id)
			}
		}
	}

	// Project and zone
	if v.Projectid != "" {
		obs.Project = v.Projectid
	}

	// Volumes: list volumes attached to the VM if we have CloudStack ID
	if vm.CloudStackID != "" {
		vp := c.csClient.Volume.NewListVolumesParams()
		vp.SetVirtualmachineid(vm.CloudStackID)
		volResp, verr := c.csClient.Volume.ListVolumes(vp)
		if verr == nil && volResp != nil && len(volResp.Volumes) > 0 {
			obs.Volumes = []v1.VolumeSpec{}
			for _, vol := range volResp.Volumes {
				vs := v1.VolumeSpec{}
				// ID and Name
				if vol.Id != "" {
					vs.ID = vol.Id
				}
				if vol.Name != "" {
					vs.Name = vol.Name
				}
				if vol.Diskofferingname != "" {
					vs.DiskOffering = vol.Diskofferingname
				} else if vol.Diskofferingid != "" {
					vs.DiskOffering = vol.Diskofferingid
				}
				// Size is returned as int64 (assume bytes); convert to GB when present
				if vol.Size > 0 {
					vs.SizeGB = int(vol.Size / (1 << 30))
					if vs.SizeGB == 0 {
						vs.SizeGB = 1
					}
				}
				obs.Volumes = append(obs.Volumes, vs)
			}
		}
	}

	// Record observed state
	if v.State != "" {
		vm.Status.ObservedState = v.State
	}

	vm.ObservedSpec = obs
	vm.Status.LastChecked = time.Now()

	return db.DB.Save(vm).Error
}

// createComponentVMs creates VM replicas for a component
func (c *Controller) createComponentVMs(appName string, comp *v1.Component, compRef v1.ComponentRef) error {
	// Determine effective VM spec: if the ComponentRef does not reference
	// a reusable VirtualMachineSpec, use the Component's persisted
	// EffectiveSpec directly. Otherwise load the referenced spec and merge
	// component-level overrides.
	overrides := comp.Spec.Overrides

	var effective v1.VirtualMachineSpec
	if compRef.VirtualMachineSpec == "" {
		effective = comp.EffectiveSpec
	} else {
		var vsr v1.VirtualMachineSpecResource
		if err := db.DB.Where("name = ?", compRef.VirtualMachineSpec).First(&vsr).Error; err != nil {
			return fmt.Errorf("virtualMachineSpec %s not found: %w", compRef.VirtualMachineSpec, err)
		}
		base := vsr.Spec
		effective = mergeVMSpec(base, overrides)
	}

	// First, create DB placeholders for any VMs that don't exist to avoid races.
	vmNames := make([]string, 0, compRef.Replicas)
	for i := 0; i < compRef.Replicas; i++ {
		vmNames = append(vmNames, fmt.Sprintf("%s-%d", comp.Metadata.Name, i+1))
	}

	vms := make([]*v1.VirtualMachine, 0, len(vmNames))
	for _, vmName := range vmNames {
		var existing v1.VirtualMachine
		if err := db.DB.Where("name = ?", vmName).First(&existing).Error; err == nil {
			// already exists, add pointer to slice
			e := existing
			vms = append(vms, &e)
			continue
		}
		// create placeholder record
		comp.EffectiveSpec = effective
		vm := &v1.VirtualMachine{
			APIVersion: v1.APIVersion,
			Kind:       "VirtualMachine",
			Metadata:   v1.Metadata{Name: vmName},
			Spec:       effective,
			Component:  comp.Metadata.Name,
		}
		if appName != "" {
			vm.Application = appName
		}
		if err := db.DB.Create(vm).Error; err != nil {
			return err
		}
		vms = append(vms, vm)
	}

	// Create VMs in parallel (limit concurrency with a semaphore)
	var wg sync.WaitGroup
	errCh := make(chan error, len(vms))
	sem := make(chan struct{}, 5) // max 5 concurrent creations

	for _, vm := range vms {
		// if VM already has CloudStackID, skip
		if vm.CloudStackID != "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(vmName string) {
			defer wg.Done()
			defer func() { <-sem }()

			// reload the VM record to get any external updates
			var cur v1.VirtualMachine
			if err := db.DB.Where("name = ?", vmName).First(&cur).Error; err != nil {
				errCh <- err
				return
			}

			id, err := handlers.ApplyVirtualMachineManaged(&cur, true)
			if err != nil {
				// cleanup placeholder to avoid repeated attempts
				_ = db.DB.Where("name = ?", vmName).Delete(&v1.VirtualMachine{}).Error
				errCh <- err
				return
			}
			if id != "" {
				if err := db.DB.Model(&v1.VirtualMachine{}).Where("name = ?", vmName).Update("cloudstack_id", id).Error; err != nil {
					errCh <- err
					return
				}
			}
		}(vm.Metadata.Name)
	}
	wg.Wait()
	close(errCh)

	// If any creation failed, return first error
	for e := range errCh {
		if e != nil {
			return e
		}
	}

	// After creating/ensuring VMs, update observed replica count and persist component effective spec
	var count int64
	if err := db.DB.Model(&v1.VirtualMachine{}).Where("name LIKE ?", comp.Metadata.Name+"-%").Count(&count).Error; err == nil {
		comp.ObservedReplicas = int(count)
	}
	// If the component is not being removed, mark it Started/Ready now that
	// its VMs have been created/ensured. This mirrors Application state.
	if comp.Status.ObservedState != "Removing" {
		comp.Status.Ready = false // not healthy yet, just started
		comp.Status.ObservedState = "Started"
		comp.Status.LastChecked = time.Now()
	}
	if err := db.DB.Save(comp).Error; err != nil {
		return err
	}

	return nil
}

// recreateComponentVMs recreates unhealthy VMs in a component
func (c *Controller) recreateComponentVMs(appName string, comp *v1.Component) error {
	// For now, simple recreation: delegate to createComponentVMs using the component spec
	compRef := v1.ComponentRef{
		Name:               comp.Metadata.Name,
		VirtualMachineSpec: comp.Spec.VirtualMachineSpec,
		Replicas:           comp.Spec.Replicas,
	}
	return c.createComponentVMs(appName, comp, compRef)
}

// mergeVMSpec merges allowed overrides into a base VirtualMachineSpec
func mergeVMSpec(base v1.VirtualMachineSpec, ov v1.ComponentOverrides) v1.VirtualMachineSpec {
	// copy base
	out := base

	// Merge user data references (append, avoid duplicates)
	if len(ov.UserDataRefs) > 0 {
		exist := map[string]bool{}
		for _, u := range out.UserDataRefs {
			exist[u] = true
		}
		for _, u := range ov.UserDataRefs {
			if !exist[u] {
				out.UserDataRefs = append(out.UserDataRefs, u)
			}
		}
	}

	// Merge SSH keys (append if provided)
	if len(ov.SSHKeys) > 0 {
		// avoid duplicates
		existing := map[string]bool{}
		for _, k := range out.SSHKeys {
			existing[k] = true
		}
		for _, k := range ov.SSHKeys {
			if !existing[k] {
				out.SSHKeys = append(out.SSHKeys, k)
			}
		}
	}

	// Merge security groups
	if len(ov.SecurityGroups) > 0 {
		existing := map[string]bool{}
		for _, s := range out.SecurityGroups {
			existing[s] = true
		}
		for _, s := range ov.SecurityGroups {
			if !existing[s] {
				out.SecurityGroups = append(out.SecurityGroups, s)
			}
		}
	}

	// Merge affinity groups
	if len(ov.AffinityGroups) > 0 {
		existing := map[string]bool{}
		for _, a := range out.AffinityGroups {
			existing[a] = true
		}
		for _, a := range ov.AffinityGroups {
			if !existing[a] {
				out.AffinityGroups = append(out.AffinityGroups, a)
			}
		}
	}

	// Override template and service offering if provided
	if ov.Template != "" {
		out.Template = ov.Template
	}
	if ov.ServiceOffering != "" {
		out.ServiceOffering = ov.ServiceOffering
	}

	// Override volumes if provided (replace)
	if len(ov.Volumes) > 0 {
		out.Volumes = ov.Volumes
	}

	return out
}
