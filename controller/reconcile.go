package controller

import (
	"fmt"
	"log"
	"strings"
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
}

// ReconcileApplication ensures application state matches desired state
func (c *Controller) ReconcileApplication(app *v1.Application) error {
	// Skip applications that are marked for removal
	if app.Status.ObservedState == "Removing" {
		return nil
	}

	return c.ResolveComponentDependencies(app)
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

	for i := 0; i < compRef.Replicas; i++ {
		vmName := fmt.Sprintf("%s-%d", comp.Metadata.Name, i+1)

		// Skip if VM already exists
		var existing v1.VirtualMachine
		if err := db.DB.Where("name = ?", vmName).First(&existing).Error; err == nil {
			// VM already present, skip creation
			continue
		}

		// persist effective spec into the component for visibility
		comp.EffectiveSpec = effective

		vm := &v1.VirtualMachine{
			APIVersion: v1.APIVersion,
			Kind:       "VirtualMachine",
			Metadata:   v1.Metadata{Name: vmName},
			Spec:       effective,
		}

		// Link VM to owning application if provided
		if appName != "" {
			vm.Application = appName
		}

		// Link VM to owning component
		vm.Component = comp.Metadata.Name

		// Persist desired VM record and create in CloudStack
		if err := db.DB.Save(vm).Error; err != nil {
			return err
		}

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

	// After creating/ensuring VMs, update observed replica count and persist component effective spec
	var count int64
	if err := db.DB.Model(&v1.VirtualMachine{}).Where("name LIKE ?", comp.Metadata.Name+"-%").Count(&count).Error; err == nil {
		comp.ObservedReplicas = int(count)
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
