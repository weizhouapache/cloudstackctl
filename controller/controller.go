package controller

import (
	"bytes"
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"cloudstackctl/pkg/handlers"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

// Controller manages CloudStack resource reconciliation and lifecycle
type Controller struct {
	csClient *cloudstack.CloudStackClient
}

// New creates a new Controller using the provided CloudStack client
func New(client *cloudstack.CloudStackClient) *Controller {
	return &Controller{csClient: client}
}

// Start launches the HTTP control plane and reconciliation loop
func (c *Controller) Start() {
	log.Println("Starting cloudstackctl controller")

	// Start HTTP server for control endpoints
	go func() {
		http.HandleFunc("/health", c.handleHealth)
		http.HandleFunc("/apply", c.handleApply)
		http.HandleFunc("/reconcile", c.handleReconcile)
		http.HandleFunc("/status", c.handleStatus)
		http.HandleFunc("/list", c.handleList)
		http.HandleFunc("/describe", c.handleDescribe)
		http.HandleFunc("/delete", c.handleDelete)

		log.Println("Controller HTTP server listening on :65426")
		if err := http.ListenAndServe(":65426", nil); err != nil {
			log.Printf("Controller HTTP server error: %v", err)
		}
	}()

	// Run reconciliation every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.ReconcileAll()
	}
}

// HTTP handler methods
func (c *Controller) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (c *Controller) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(body, &meta); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	kind, _ := meta["kind"].(string)
	var applyErr error
	var appliedID string
	var appliedOp string // "created" | "updated" | "accepted"

	switch kind {
	case "VirtualMachineSpec":
		var vs v1.VirtualMachineSpecResource
		if err := json.Unmarshal(body, &vs); err != nil {
			http.Error(w, "failed to parse VirtualMachineSpec", http.StatusBadRequest)
			return
		}
		// detect create vs update
		if db.DB == nil {
			appliedOp = "accepted"
		} else {
			var existing v1.VirtualMachineSpecResource
			if db.DB.Where("name = ?", vs.Metadata.Name).First(&existing).Error != nil {
				appliedOp = "created"
			} else {
				appliedOp = "updated"
			}
		}
		applyErr = c.applyVMSpec(&vs)
	case "Application":
		var app v1.Application
		if err := json.Unmarshal(body, &app); err != nil {
			http.Error(w, "failed to parse Application", http.StatusBadRequest)
			return
		}
		applyErr = c.applyApplication(&app)
	case "Component":
		// Accept two forms for Component.spec.virtualMachineSpec:
		// - a string referencing a named VirtualMachineSpec
		// - an inline object describing a VirtualMachineSpec
		type compSpecIn struct {
			VirtualMachineSpec json.RawMessage       `json:"virtualMachineSpec"`
			Replicas           int                   `json:"replicas"`
			Overrides          v1.ComponentOverrides `json:"overrides"`
			HealthChecks       []v1.HealthCheck      `json:"healthChecks"`
		}
		type compIn struct {
			APIVersion string      `json:"apiVersion"`
			Kind       string      `json:"kind"`
			Metadata   v1.Metadata `json:"metadata"`
			Spec       compSpecIn  `json:"spec"`
			Status     v1.Status   `json:"status"`
		}

		var ci compIn
		if err := json.Unmarshal(body, &ci); err != nil {
			http.Error(w, "failed to parse Component", http.StatusBadRequest)
			return
		}

		comp := v1.Component{
			APIVersion: ci.APIVersion,
			Kind:       ci.Kind,
			Metadata:   ci.Metadata,
			Spec: v1.ComponentSpec{
				Replicas:     ci.Spec.Replicas,
				Overrides:    ci.Spec.Overrides,
				HealthChecks: ci.Spec.HealthChecks,
			},
		}

		// Interpret VirtualMachineSpec field which may be a string or object
		if len(ci.Spec.VirtualMachineSpec) > 0 {
			// If it starts with a quote it's a JSON string
			b := ci.Spec.VirtualMachineSpec
			// trim whitespace
			trimmed := bytes.TrimSpace(b)
			if len(trimmed) > 0 && trimmed[0] == '"' {
				var ref string
				if err := json.Unmarshal(b, &ref); err == nil {
					comp.Spec.VirtualMachineSpec = ref
				}
			} else {
				// Attempt to decode inline VirtualMachineSpec
				var vms v1.VirtualMachineSpec
				if err := json.Unmarshal(b, &vms); err == nil {
					comp.EffectiveSpec = vms
				}
			}
		}

		// detect create vs update
		if db.DB == nil {
			appliedOp = "accepted"
		} else {
			var existing v1.Component
			if db.DB.Where("name = ?", comp.Metadata.Name).First(&existing).Error != nil {
				appliedOp = "created"
			} else {
				appliedOp = "updated"
			}
		}
		applyErr = c.applyComponent(&comp)
	case "VirtualMachine":
		var vm v1.VirtualMachine
		if err := json.Unmarshal(body, &vm); err != nil {
			http.Error(w, "failed to parse VirtualMachine", http.StatusBadRequest)
			return
		}
		applyErr = c.applyVM(&vm)
	case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData":
		appliedID, applyErr = handlers.ApplyCloudStackResource(body)
		if applyErr == nil && appliedID != "" {
			log.Printf("Applied %s id=%s", kind, appliedID)
		}
	default:
		http.Error(w, "unsupported kind", http.StatusBadRequest)
		return
	}

	if applyErr != nil {
		log.Printf("apply error for kind=%s: %v", kind, applyErr)
		http.Error(w, fmt.Sprintf("failed to apply resource: %v", applyErr), http.StatusInternalServerError)
		return
	}

	// Build response including created/applied resource id when available
	respMap := map[string]string{"status": "success", "message": "resource accepted for reconciliation"}
	if appliedOp != "" {
		respMap["action"] = appliedOp
		switch appliedOp {
		case "created":
			respMap["message"] = "resource created"
		case "updated":
			respMap["message"] = "resource updated"
		}
	}
	if appliedID != "" {
		respMap["id"] = appliedID
		respMap["kind"] = kind
		respMap["message"] = "resource applied"
	}
	b, _ := json.Marshal(respMap)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func (c *Controller) handleReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]string
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	kind := payload["kind"]
	name := payload["name"]

	switch kind {
	case "Component":
		var comp v1.Component
		if err := db.DB.Where("name = ?", name).First(&comp).Error; err != nil {
			http.Error(w, "component not found", http.StatusNotFound)
			return
		}
		if err := c.ReconcileComponent(&comp); err != nil {
			log.Printf("reconcile component %s failed: %v", name, err)
			http.Error(w, "reconcile failed", http.StatusInternalServerError)
			return
		}
	case "VirtualMachine":
		var vm v1.VirtualMachine
		if err := db.DB.Where("name = ?", name).First(&vm).Error; err != nil {
			http.Error(w, "virtualmachine not found", http.StatusNotFound)
			return
		}
		if err := c.ReconcileVM(&vm); err != nil {
			log.Printf("reconcile vm %s failed: %v", name, err)
			http.Error(w, "reconcile failed", http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "unsupported kind", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"reconcile triggered"}`))
}

func (c *Controller) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	if kind == "Application" {
		var app v1.Application
		if err := db.DB.Where("name = ?", name).First(&app).Error; err != nil {
			http.Error(w, "application not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(app)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	}
	if kind == "Component" {
		var comp v1.Component
		if err := db.DB.Where("name = ?", name).First(&comp).Error; err != nil {
			http.Error(w, "component not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(comp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	}
	if kind == "VirtualMachine" {
		var vm v1.VirtualMachine
		if err := db.DB.Where("name = ?", name).First(&vm).Error; err != nil {
			http.Error(w, "virtualmachine not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(vm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	}
	http.Error(w, "unsupported kind", http.StatusBadRequest)
}

func (c *Controller) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	switch kind {
	case "Application":
		var apps []v1.Application
		if db.DB == nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := db.DB.Find(&apps).Error; err != nil {
			http.Error(w, "failed to list applications", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(apps)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "Component":
		var comps []v1.Component
		if db.DB == nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := db.DB.Find(&comps).Error; err != nil {
			http.Error(w, "failed to list components", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(comps)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "VirtualMachine":
		if r.URL.Query().Get("all") == "true" {
			payload := map[string]string{"kind": "VirtualMachine"}
			if name != "" {
				payload["name"] = name
			}
			raw, _ := json.Marshal(payload)
			obj, err := handlers.GetCloudStackResource(raw)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list VirtualMachine: %v", err), http.StatusInternalServerError)
				return
			}
			b, _ := json.Marshal(obj)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(b)
			return
		}

		var vms []v1.VirtualMachine
		if db.DB == nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := db.DB.Find(&vms).Error; err != nil {
			http.Error(w, "failed to list virtualmachines", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(vms)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "VirtualMachineSpec":
		var specs []v1.VirtualMachineSpecResource
		if db.DB == nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := db.DB.Find(&specs).Error; err != nil {
			http.Error(w, "failed to list virtualmachinespecs", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(specs)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return

	case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData", "Template":
		payload := map[string]string{"kind": kind}
		if name != "" {
			payload["name"] = name
		}
		raw, _ := json.Marshal(payload)
		obj, err := handlers.GetCloudStackResource(raw)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list %s: %v", kind, err), http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(obj)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	default:
		http.Error(w, "unsupported kind", http.StatusBadRequest)
		return
	}
}

func (c *Controller) handleDescribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	if name == "" || kind == "" {
		http.Error(w, "missing kind or name", http.StatusBadRequest)
		return
	}
	switch kind {
	case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData":
		payload := map[string]string{"kind": kind, "name": name}
		raw, _ := json.Marshal(payload)
		if resp, err := handlers.DescribeCloudStackResource(raw); err != nil {
			log.Fatalf("Local describe failed: %v", err)
		} else {
			b, _ := json.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(b)
			return
		}
	case "VirtualMachineSpec":
		var spec v1.VirtualMachineSpecResource
		if db.DB == nil || db.DB.Where("name = ?", name).First(&spec).Error != nil {
			http.Error(w, "virtualmachinespec not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(spec)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "Application":
		var app v1.Application
		if db.DB == nil || db.DB.Where("name = ?", name).First(&app).Error != nil {
			http.Error(w, "application not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(app)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "Component":
		var comp v1.Component
		if db.DB == nil || db.DB.Where("name = ?", name).First(&comp).Error != nil {
			http.Error(w, "component not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(comp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "VirtualMachine":
		if r.URL.Query().Get("all") == "true" {
			payload := map[string]string{"kind": kind}
			if name != "" {
				payload["name"] = name
			}
			raw, _ := json.Marshal(payload)
			obj, err := handlers.DescribeCloudStackResource(raw)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to describe %s: %v", kind, err), http.StatusInternalServerError)
				return
			}
			b, _ := json.Marshal(obj)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(b)
			return
		}
		var vm v1.VirtualMachine
		if db.DB == nil || db.DB.Where("name = ?", name).First(&vm).Error != nil {
			http.Error(w, "virtualmachine not found", http.StatusNotFound)
			return
		}
		b, _ := json.Marshal(vm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	default:
		http.Error(w, "unsupported kind", http.StatusBadRequest)
		return
	}
}

func (c *Controller) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}
	kind := payload["kind"]
	name := payload["name"]
	switch kind {
	case "Application":
		var app v1.Application
		if db.DB == nil || db.DB.Where("name = ?", name).First(&app).Error != nil {
			http.Error(w, "application not found", http.StatusNotFound)
			return
		}
		for _, cref := range app.Spec.Components {
			db.DB.Where("name = ?", cref.Name).Delete(&v1.Component{})
		}
		db.DB.Delete(&app)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"deleted"}`))
		return
	case "Component":
		db.DB.Where("name = ?", name).Delete(&v1.Component{})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"deleted"}`))
		return
	case "VirtualMachineSpec":
		var spec v1.VirtualMachineSpecResource
		if db.DB == nil || db.DB.Where("name = ?", name).First(&spec).Error != nil {
			http.Error(w, "virtualmachinespec not found", http.StatusNotFound)
			return
		}
		db.DB.Delete(&spec)
		respMap := map[string]string{"status": "deleted", "kind": "VirtualMachineSpec", "name": name}
		b, _ := json.Marshal(respMap)
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "VirtualMachine":
		var vm v1.VirtualMachine
		if db.DB == nil || db.DB.Where("name = ?", name).First(&vm).Error != nil {
			params := c.csClient.VirtualMachine.NewListVirtualMachinesParams()
			params.SetName(name)
			resp, _ := c.csClient.VirtualMachine.ListVirtualMachines(params)
			if resp != nil && len(resp.VirtualMachines) > 0 {
				id := resp.VirtualMachines[0].Id
				dp := c.csClient.VirtualMachine.NewDestroyVirtualMachineParams(id)
				dp.SetExpunge(true)
				c.csClient.VirtualMachine.DestroyVirtualMachine(dp)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"deleted"}`))
				return
			}
			http.Error(w, "virtualmachine not found", http.StatusNotFound)
			return
		}
		if vm.Status.CloudStackID != "" {
			dp := c.csClient.VirtualMachine.NewDestroyVirtualMachineParams(vm.Status.CloudStackID)
			c.csClient.VirtualMachine.DestroyVirtualMachine(dp)
		}
		db.DB.Delete(&vm)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"deleted"}`))
		return
	case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData", "Template", "Snapshot":
		// Delegate deletes of unmanaged CloudStack resources to handlers
		if id, err := handlers.DeleteCloudStackResource(body); err != nil {
			http.Error(w, fmt.Sprintf("failed to delete %s: %v", kind, err), http.StatusInternalServerError)
			return
		} else {
			if id != "" {
				log.Printf("Deleted %s id=%s", kind, id)
			}
			respMap := map[string]string{"status": "deleted"}
			if id != "" {
				respMap["id"] = id
				respMap["kind"] = kind
			}
			b, _ := json.Marshal(respMap)
			w.WriteHeader(http.StatusOK)
			w.Write(b)
			return
		}
	default:
		http.Error(w, "unsupported kind for delete", http.StatusBadRequest)
		return
	}
}

// Apply creates/updates a resource in CloudStack
func (c *Controller) Apply(resource interface{}) error {
	switch res := resource.(type) {
	case *v1.Application:
		return c.applyApplication(res)
	case *v1.Component:
		return c.applyComponent(res)
	case *v1.VirtualMachineSpecResource:
		return c.applyVMSpec(res)
	case *v1.VirtualMachine:
		// Default to immediate apply for VirtualMachine resources
		_, err := handlers.ApplyVirtualMachineManaged(res, true)
		if err != nil {
			return err
		}
		return nil
	case *v1.Network:
		if _, err := handlers.ApplyNetwork(res); err != nil {
			return err
		}
		return nil
	case *v1.Volume:
		if _, err := handlers.ApplyVolume(res); err != nil {
			return err
		}
		return nil
	case *v1.SSHKey:
		if _, err := handlers.ApplySSHKey(res); err != nil {
			return err
		}
		return nil
	case *v1.SecurityGroup:
		if _, err := handlers.ApplySecurityGroup(res); err != nil {
			return err
		}
		return nil
	case *v1.AffinityGroup:
		if _, err := handlers.ApplyAffinityGroup(res); err != nil {
			return err
		}
		return nil
	case *v1.UserData:
		if _, err := handlers.ApplyUserData(res); err != nil {
			return err
		}
		return nil
	default:
		return logError("Unsupported resource type: %T", res)
	}
}

// applyApplication creates/updates an Application resource
func (c *Controller) applyApplication(app *v1.Application) error {
	// Save desired state to database
	if err := db.DB.Save(app).Error; err != nil {
		return err
	}

	// Resolve component dependencies and create resources
	return c.ResolveComponentDependencies(app)
}

// applyComponent creates/updates a Component resource
func (c *Controller) applyComponent(comp *v1.Component) error {
	return db.DB.Save(comp).Error
}

// applyVMSpec creates/updates a reusable VirtualMachineSpec resource
func (c *Controller) applyVMSpec(vs *v1.VirtualMachineSpecResource) error {
	// Only allow create (apply) or idempotent re-apply with identical spec.
	var existing v1.VirtualMachineSpecResource
	if err := db.DB.Where("name = ?", vs.Metadata.Name).First(&existing).Error; err == nil {
		// already exists
		if reflect.DeepEqual(existing.Spec, vs.Spec) {
			return nil
		}
		return fmt.Errorf("VirtualMachineSpec %s already exists and cannot be updated", vs.Metadata.Name)
	}
	return db.DB.Create(vs).Error
}

// applyVM creates/updates a VirtualMachine resource
func (c *Controller) applyVM(vm *v1.VirtualMachine) error {
	// Attempt to find the VM in CloudStack by CloudStackID or by name
	// If CloudStackID is empty, try to discover VM in CloudStack
	if vm.Status.CloudStackID == "" {
		// search CloudStack by name and project
		params := c.csClient.VirtualMachine.NewListVirtualMachinesParams()
		params.SetName(vm.Metadata.Name)
		if vm.Spec.Project != "" {
			if pid, perr := handlers.ResolveProject(vm.Spec.Project); perr == nil {
				params.SetProjectid(pid)
			} else {
				params.SetProjectid(vm.Spec.Project)
			}
		}
		tags := map[string]string{"managed_by": "cloudstackctl"}
		params.SetTags(tags)
		resp, err := c.csClient.VirtualMachine.ListVirtualMachines(params)
		if err == nil && resp != nil && len(resp.VirtualMachines) > 0 {
			// associate existing CloudStack VM
			vm.Status.CloudStackID = resp.VirtualMachines[0].Id
			vm.Status.ObservedState = resp.VirtualMachines[0].State
		}
	}

	// Persist desired state to DB (create or update)
	var existing v1.VirtualMachine
	if err := db.DB.Where("name = ?", vm.Metadata.Name).First(&existing).Error; err != nil {
		// record not found: create new record with observed CloudStack info
		if err := db.DB.Save(vm).Error; err != nil {
			return err
		}
		return nil
	}

	// Compare specs for drift detection
	if !compareVMSpec(existing.Spec, vm.Spec) {
		existing.Status.Drift = true
		// Optionally record which fields differ (simple approach: store as annotation)
		if existing.Metadata.Annotations == nil {
			existing.Metadata.Annotations = map[string]string{}
		}
		existing.Metadata.Annotations["drift.reason"] = "spec mismatch"
	}

	// Update CloudStackID and observed state if discovered
	if vm.Status.CloudStackID != "" {
		existing.Status.CloudStackID = vm.Status.CloudStackID
		existing.Status.ObservedState = vm.Status.ObservedState
	}

	// Save updates
	return db.DB.Save(&existing).Error
}

// compareVMSpec does a basic comparison of VM specs to detect drift
func compareVMSpec(a, b v1.VirtualMachineSpec) bool {
	if a.Template != b.Template {
		return false
	}
	if a.ServiceOffering != b.ServiceOffering {
		return false
	}
	if a.Project != b.Project {
		return false
	}
	if len(a.Networks) != len(b.Networks) {
		return false
	}
	for i := range a.Networks {
		if a.Networks[i] != b.Networks[i] {
			return false
		}
	}
	return true
}

// applyNetwork creates/updates a Network resource
func (c *Controller) applyNetwork(net *v1.Network) error {
	return db.DB.Save(net).Error
}

// applyVolume creates/updates a Volume resource
func (c *Controller) applyVolume(vol *v1.Volume) error {
	return db.DB.Save(vol).Error
}

// applySSHKey creates/updates an SSHKey resource
func (c *Controller) applySSHKey(key *v1.SSHKey) error {
	return db.DB.Save(key).Error
}

// applySecurityGroup creates/updates a SecurityGroup resource
func (c *Controller) applySecurityGroup(sg *v1.SecurityGroup) error {
	return db.DB.Save(sg).Error
}

// applyAffinityGroup creates/updates an AffinityGroup resource
func (c *Controller) applyAffinityGroup(ag *v1.AffinityGroup) error {
	return db.DB.Save(ag).Error
}

// applyUserData creates/updates a UserData resource
func (c *Controller) applyUserData(ud *v1.UserData) error {
	return db.DB.Save(ud).Error
}
