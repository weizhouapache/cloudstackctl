package controller

import (
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
	csClient *cloudstack.CloudStackClient // CloudStack SDK client
}

// New creates a new Controller instance
func New(csClient *cloudstack.CloudStackClient) *Controller {
	// Ensure DB is initialized and tables exist (controller can be started without separate DB init)
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Printf("Warning: failed to initialize DB in controller.New: %v", err)
		}
	}

	// Ensure required controller-managed tables exist (idempotent).
	// Rely on model `TableName()` overrides for stable table naming.
	if db.DB != nil {
		if err := db.DB.AutoMigrate(&v1.Application{}, &v1.Component{}, &v1.VirtualMachineSpecResource{}, &v1.VirtualMachine{}); err != nil {
			log.Printf("Warning: failed to auto-migrate controller tables: %v", err)
		}
	}

	return &Controller{
		csClient: csClient,
	}
}

// Start runs the controller reconciliation loop
func (c *Controller) Start() {
	log.Println("Starting cloudstackctl controller")

	// Start a lightweight HTTP server for health checks and status on port 65426
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		// Accept resource apply requests from the CLI in cluster mode
		http.HandleFunc("/apply", func(w http.ResponseWriter, r *http.Request) {
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

			switch kind {
			case "VirtualMachineSpec":
				var vs v1.VirtualMachineSpecResource
				if err := json.Unmarshal(body, &vs); err != nil {
					http.Error(w, "failed to parse VirtualMachineSpec", http.StatusBadRequest)
					return
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
				var comp v1.Component
				if err := json.Unmarshal(body, &comp); err != nil {
					http.Error(w, "failed to parse Component", http.StatusBadRequest)
					return
				}
				applyErr = c.applyComponent(&comp)
			case "VirtualMachine":
				// Check whether this VirtualMachine references a reusable VM spec
				// If it does (spec.virtualMachineSpec present) treat as managed and
				// persist to DB; otherwise treat as unmanaged and apply immediately.
				var raw map[string]interface{}
				if err := json.Unmarshal(body, &raw); err != nil {
					http.Error(w, "failed to parse VirtualMachine", http.StatusBadRequest)
					return
				}
				var vm v1.VirtualMachine
				if err := json.Unmarshal(body, &vm); err != nil {
					http.Error(w, "failed to parse VirtualMachine", http.StatusBadRequest)
					return
				}
				applyErr = c.applyVM(&vm)
			case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData":
				// Use shared apply wrapper for unmanaged CloudStack resources
				applyErr = handlers.ApplyCloudStackResource(body)
			default:
				http.Error(w, "unsupported kind", http.StatusBadRequest)
				return
			}

			if applyErr != nil {
				log.Printf("apply error for kind=%s: %v", kind, applyErr)
				http.Error(w, "failed to apply resource", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"success","message":"resource accepted for reconciliation"}`))
		})

		// Force reconcile endpoint: POST /reconcile {"kind":"Component","name":"my-comp"}
		http.HandleFunc("/reconcile", func(w http.ResponseWriter, r *http.Request) {
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
				if err := db.DB.Where("metadata_name = ?", name).First(&comp).Error; err != nil {
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
				if err := db.DB.Where("metadata_name = ?", name).First(&vm).Error; err != nil {
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
		})

		// Status endpoint: GET /status?kind=Component|VirtualMachine&name=<name>
		http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			kind := r.URL.Query().Get("kind")
			name := r.URL.Query().Get("name")
			if kind == "Application" {
				var app v1.Application
				if err := db.DB.Where("metadata_name = ?", name).First(&app).Error; err != nil {
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
				if err := db.DB.Where("metadata_name = ?", name).First(&comp).Error; err != nil {
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
				if err := db.DB.Where("metadata_name = ?", name).First(&vm).Error; err != nil {
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
		})

		// List endpoint: GET /list?kind=<Kind>
		http.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
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
				// If client requested all VMs, delegate to handlers which will
				// query CloudStack directly. This preserves parity with
				// standalone mode when `--all` is used in the CLI.
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
			// Unmanaged kinds: delegate to handlers.GetCloudStackResource which
			// centralizes CloudStack listing logic. Controller should not call
			// CloudStack APIs directly.
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
		})

		// Describe endpoint: GET /describe?kind=&name=
		http.HandleFunc("/describe", func(w http.ResponseWriter, r *http.Request) {
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
				// Standalone: use local describe wrapper
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
			case "Application":
				var app v1.Application
				if db.DB == nil || db.DB.Where("metadata_name = ?", name).First(&app).Error != nil {
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
				if db.DB == nil || db.DB.Where("metadata_name = ?", name).First(&comp).Error != nil {
					http.Error(w, "component not found", http.StatusNotFound)
					return
				}
				b, _ := json.Marshal(comp)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(b)
				return
			case "VirtualMachine":
				// If client asked for all, delegate to handlers which query CloudStack
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
				if db.DB == nil || db.DB.Where("metadata_name = ?", name).First(&vm).Error != nil {
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
		})

		// Delete endpoint: POST /delete {"kind":"...","name":"..."}
		http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
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
				if db.DB == nil || db.DB.Where("metadata_name = ?", name).First(&app).Error != nil {
					http.Error(w, "application not found", http.StatusNotFound)
					return
				}
				// delete dependent components
				for _, cref := range app.Spec.Components {
					db.DB.Where("metadata_name = ?", cref.Name).Delete(&v1.Component{})
				}
				db.DB.Delete(&app)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"deleted"}`))
				return
			case "Component":
				db.DB.Where("metadata_name = ?", name).Delete(&v1.Component{})
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"deleted"}`))
				return
			case "VirtualMachine":
				var vm v1.VirtualMachine
				if db.DB == nil || db.DB.Where("metadata_name = ?", name).First(&vm).Error != nil {
					// fallback: attempt CloudStack delete by name
					params := c.csClient.VirtualMachine.NewListVirtualMachinesParams()
					params.SetName(name)
					resp, _ := c.csClient.VirtualMachine.ListVirtualMachines(params)
					if resp != nil && len(resp.VirtualMachines) > 0 {
						id := resp.VirtualMachines[0].Id
						dp := c.csClient.VirtualMachine.NewDestroyVirtualMachineParams(id)
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
			default:
				http.Error(w, "unsupported kind for delete", http.StatusBadRequest)
				return
			}
		})
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
		return handlers.ApplyVirtualMachineManaged(res, true)
	case *v1.Network:
		return handlers.ApplyNetwork(res)
	case *v1.Volume:
		return handlers.ApplyVolume(res)
	case *v1.SSHKey:
		return handlers.ApplySSHKey(res)
	case *v1.SecurityGroup:
		return handlers.ApplySecurityGroup(res)
	case *v1.AffinityGroup:
		return handlers.ApplyAffinityGroup(res)
	case *v1.UserData:
		return handlers.ApplyUserData(res)
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
	if err := db.DB.Where("metadata_name = ?", vs.Metadata.Name).First(&existing).Error; err == nil {
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
	if err := db.DB.Where("metadata ->> 'name' = ?", vm.Metadata.Name).First(&existing).Error; err != nil {
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
	if len(a.NetworkIDs) != len(b.NetworkIDs) {
		return false
	}
	for i := range a.NetworkIDs {
		if a.NetworkIDs[i] != b.NetworkIDs[i] {
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
