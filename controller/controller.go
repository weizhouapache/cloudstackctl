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
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/apache/cloudstack-go/v2/cloudstack"
)

// Controller manages CloudStack resource reconciliation and lifecycle
type Controller struct {
	csClient   *cloudstack.CloudStackClient
	mu         sync.Mutex
	appTickers map[string]*time.Ticker
	appQuit    map[string]chan struct{}
}

// startAppWorker starts a per-application reconcile ticker if not already running.
func (c *Controller) startAppWorker(appName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.appTickers[appName]; ok {
		return
	}
	t := time.NewTicker(10 * time.Second)
	quit := make(chan struct{})
	c.appTickers[appName] = t
	c.appQuit[appName] = quit

	go func(name string, tk *time.Ticker, q chan struct{}) {
		log.Println("Starting reconciliation loop for application:", name)
		for {
			select {
			case <-tk.C:
				var app v1.Application
				if err := db.DB.Where("name = ?", name).First(&app).Error; err != nil {
					log.Printf("app worker: application %s not found: %v", name, err)
					continue
				}
				switch app.Status.ObservedState {
				case "Removing":
					// If application is marked Removing, perform scoped removal in order: VMs -> Components -> Application
					log.Printf("app worker: processing removal for application %s", name)

					if err := c.ReconcileRemovingApplication(&app); err != nil {
						log.Printf("app worker: removing application %s failed: %v", name, err)
						continue
					}
					return

				case "Starting":
					// Start the application
					log.Printf("app worker: starting application %s", name)
					if err := c.ResolveComponentDependencies(&app); err != nil {
						log.Printf("app worker: start application %s failed: %v", name, err)
					}
					continue

				default:
					// Reconcile the application
					log.Printf("app worker: reconciling application %s", name)
					if err := c.ReconcileApplication(&app); err != nil {
						log.Printf("app worker: reconcile application %s failed: %v", name, err)
					}
					continue
				}
			case <-q:
				tk.Stop()
				return
			}
		}
	}(appName, t, quit)
}

// stopAppWorker stops and removes the per-application worker if present.
func (c *Controller) stopAppWorker(appName string) {
	// Remove ticker and quit channel under lock, then perform cleanup
	c.mu.Lock()
	q, qok := c.appQuit[appName]
	t, tok := c.appTickers[appName]
	if qok {
		select {
		default:
			close(q)
		}
		delete(c.appQuit, appName)
	}
	if tok {
		t.Stop()
		delete(c.appTickers, appName)
	}
	c.mu.Unlock()

	// Proceed to remove resources for the application in order: VMs -> Components -> Application
	// Load application to obtain component refs
	var app v1.Application
	if err := db.DB.Where("name = ?", appName).First(&app).Error; err != nil {
		log.Printf("stopAppWorker: application %s not found for cleanup: %v", appName, err)
		return
	}

	// 1) Delete VMs referencing this application, grouped by component.
	var appVMs []v1.VirtualMachine
	if err := db.DB.Where("application = ?", appName).Find(&appVMs).Error; err != nil {
		log.Printf("stopAppWorker: failed to list VMs for app %s: %v", appName, err)
	} else {
		// Group VMs by component name (empty string for none)
		vmsByComp := make(map[string][]v1.VirtualMachine)
		var compOrder []string
		for _, vm := range appVMs {
			comp := vm.Component
			if _, ok := vmsByComp[comp]; !ok {
				compOrder = append(compOrder, comp)
			}
			vmsByComp[comp] = append(vmsByComp[comp], vm)
		}

		// Process components in deterministic order, deleting each component's VMs in parallel.
		// Launches are staggered 2 seconds apart to avoid concurrent CloudStack
		// API contention; goroutines themselves run concurrently after their delay.
		for _, comp := range compOrder {
			vms := vmsByComp[comp]
			log.Printf("stopAppWorker: removing %d VMs for component '%s'", len(vms), comp)
			var wg sync.WaitGroup
			for i, vm := range vms {
				if i > 0 {
					time.Sleep(2 * time.Second)
				}
				wg.Add(1)
				vmCopy := vm
				go func(v v1.VirtualMachine) {
					defer wg.Done()
					log.Printf("stopAppWorker: removing VM %s", v.Metadata.Name)
					if v.CloudStackID != "" {
						// Check if VM still exists in CloudStack before attempting deletion
						params := c.csClient.VirtualMachine.NewListVirtualMachinesParams()
						params.SetId(v.CloudStackID)
						resp, _ := c.csClient.VirtualMachine.ListVirtualMachines(params)
						if resp != nil && len(resp.VirtualMachines) > 0 {
							dp := c.csClient.VirtualMachine.NewDestroyVirtualMachineParams(v.CloudStackID)
							dp.SetExpunge(true)
							if _, err := c.csClient.VirtualMachine.DestroyVirtualMachine(dp); err != nil {
								log.Printf("stopAppWorker: failed to destroy CloudStack VM %s (id=%s): %v", v.Metadata.Name, v.CloudStackID, err)
								return
							}
						}
					}
					if err := db.DB.Delete(&v).Error; err != nil {
						log.Printf("stopAppWorker: failed to delete VM record %s: %v", v.Metadata.Name, err)
					}
				}(vmCopy)
			}
			wg.Wait()
		}
	}

	// 2) Delete components referenced by application if no VMs remain
	var compNames []string
	for _, cref := range app.Spec.Components {
		compNames = append(compNames, cref.Name)
	}
	for _, cname := range compNames {
		var vmCount int64
		if err := db.DB.Model(&v1.VirtualMachine{}).Where("component = ?", cname).Count(&vmCount).Error; err != nil {
			log.Printf("stopAppWorker: failed to count VMs for component %s: %v", cname, err)
			continue
		}
		if vmCount > 0 {
			log.Printf("stopAppWorker: skipping deletion of component %s: %d VMs still exist", cname, vmCount)
			continue
		}
		log.Printf("stopAppWorker: removing Component: %s", cname)
		if err := db.DB.Where("name = ?", cname).Delete(&v1.Component{}).Error; err != nil {
			log.Printf("stopAppWorker: failed to delete component %s: %v", cname, err)
		}
	}

	// 3) Delete application record
	if err := db.DB.Delete(&app).Error; err != nil {
		log.Printf("stopAppWorker: failed to delete application %s: %v", appName, err)
	} else {
		log.Printf("stopAppWorker: application %s removed", appName)
	}
}

// New creates a new Controller using the provided CloudStack client
func New(client *cloudstack.CloudStackClient) *Controller {
	return &Controller{
		csClient:   client,
		appTickers: make(map[string]*time.Ticker),
		appQuit:    make(map[string]chan struct{}),
	}
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

	// Run reconciliation every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
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

	dec := yaml.NewDecoder(bytes.NewReader(body))
	var results []map[string]string

	for {
		var doc interface{}
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			http.Error(w, fmt.Sprintf("failed to decode payload: %v", err), http.StatusBadRequest)
			return
		}
		if doc == nil {
			continue
		}

		raw, _ := json.Marshal(doc)
		var meta map[string]interface{}
		if err := json.Unmarshal(raw, &meta); err != nil {
			http.Error(w, "invalid resource document", http.StatusBadRequest)
			return
		}
		kind, _ := meta["kind"].(string)

		switch kind {
		case "VirtualMachineSpec":
			var vs v1.VirtualMachineSpecResource
			if err := json.Unmarshal(raw, &vs); err != nil {
				http.Error(w, "failed to parse VirtualMachineSpec", http.StatusBadRequest)
				return
			}
			appliedOp := "accepted"
			if db.DB != nil {
				var existing v1.VirtualMachineSpecResource
				if db.DB.Where("name = ?", vs.Metadata.Name).First(&existing).Error != nil {
					appliedOp = "created"
				} else {
					appliedOp = "updated"
				}
			}
			if err := c.applyVMSpec(&vs); err != nil {
				results = append(results, map[string]string{"kind": kind, "name": vs.Metadata.Name, "status": "error", "message": err.Error()})
				continue
			}
			results = append(results, map[string]string{"kind": kind, "name": vs.Metadata.Name, "status": "success", "action": appliedOp})

		case "Application":
			var app v1.Application
			if err := json.Unmarshal(raw, &app); err != nil {
				http.Error(w, "failed to parse Application", http.StatusBadRequest)
				return
			}
			if err := c.applyApplication(&app); err != nil {
				results = append(results, map[string]string{"kind": kind, "name": app.Metadata.Name, "status": "error", "message": err.Error()})
				continue
			}
			results = append(results, map[string]string{"kind": kind, "name": app.Metadata.Name, "status": "success", "action": "accepted"})

		case "Component":
			// Best-effort: unmarshal directly into Component. Inline VM spec support
			// is preserved via `Component.EffectiveSpec` elsewhere; reject malformed input here.
			var comp v1.Component
			if err := json.Unmarshal(raw, &comp); err != nil {
				http.Error(w, "failed to parse Component", http.StatusBadRequest)
				return
			}
			if err := c.applyComponent(&comp); err != nil {
				results = append(results, map[string]string{"kind": kind, "name": comp.Metadata.Name, "status": "error", "message": err.Error()})
				continue
			}
			results = append(results, map[string]string{"kind": kind, "name": comp.Metadata.Name, "status": "success", "action": "accepted"})

		case "VirtualMachine":
			var vm v1.VirtualMachine
			if err := json.Unmarshal(raw, &vm); err != nil {
				http.Error(w, "failed to parse VirtualMachine", http.StatusBadRequest)
				return
			}
			if err := c.applyVM(&vm); err != nil {
				results = append(results, map[string]string{"kind": kind, "name": vm.Metadata.Name, "status": "error", "message": err.Error()})
				continue
			}
			results = append(results, map[string]string{"kind": kind, "name": vm.Metadata.Name, "status": "success", "action": "applied"})

		case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData":
			id, err := handlers.ApplyCloudStackResource(raw)
			if err != nil {
				results = append(results, map[string]string{"kind": kind, "status": "error", "message": err.Error()})
				continue
			}
			if id != "" {
				log.Printf("Applied %s id=%s", kind, id)
				results = append(results, map[string]string{"kind": kind, "status": "success", "id": id})
			} else {
				results = append(results, map[string]string{"kind": kind, "status": "success"})
			}

		case "Project":
			id, err := handlers.ApplyCloudStackResource(raw)
			if err != nil {
				results = append(results, map[string]string{"kind": kind, "status": "error", "message": err.Error()})
				continue
			}
			results = append(results, map[string]string{"kind": kind, "status": "success", "id": id})

		default:
			http.Error(w, "unsupported kind", http.StatusBadRequest)
			return
		}
	}

	b, _ := json.Marshal(results)
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
	appFilter := r.URL.Query().Get("application")
	projectFilter := r.URL.Query().Get("project")
	allProjects := r.URL.Query().Get("all-projects") == "true"
	switch kind {
	case "Application":
		var apps []v1.Application
		if db.DB == nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		q := db.DB
		if appFilter != "" {
			q = q.Where("name = ?", appFilter)
		}
		if projectFilter != "" {
			q = q.Where("metadata_project = ?", projectFilter)
		} else if !allProjects {
			q = q.Where("metadata_project IS NULL OR metadata_project = ''")
		}
		if err := q.Find(&apps).Error; err != nil {
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
		q := db.DB
		if appFilter != "" {
			q = q.Where("application = ?", appFilter)
		}
		if projectFilter != "" {
			var appNames []string
			if err := db.DB.Model(&v1.Application{}).Where("metadata_project = ?", projectFilter).Pluck("name", &appNames).Error; err != nil {
				http.Error(w, "failed to filter components by project", http.StatusInternalServerError)
				return
			}
			if len(appNames) > 0 {
				q = q.Where("application IN ?", appNames)
			} else {
				q = q.Where("(application IS NULL OR application = '') AND metadata_project = ?", projectFilter)
			}
		} else if !allProjects {
			var appNames []string
			if err := db.DB.Model(&v1.Application{}).Where("metadata_project IS NULL OR metadata_project = ''").Pluck("name", &appNames).Error; err != nil {
				http.Error(w, "failed to filter components by default project scope", http.StatusInternalServerError)
				return
			}
			if len(appNames) > 0 {
				q = q.Where("application IN ? OR ((application IS NULL OR application = '') AND (metadata_project IS NULL OR metadata_project = ''))", appNames)
			} else {
				q = q.Where("(application IS NULL OR application = '') AND (metadata_project IS NULL OR metadata_project = '')")
			}
		}
		if err := q.Find(&comps).Error; err != nil {
			http.Error(w, "failed to list components", http.StatusInternalServerError)
			return
		}
		b, _ := json.Marshal(comps)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		return
	case "VirtualMachine":
		if r.URL.Query().Get("all-vms") == "true" {
			payload := map[string]interface{}{"kind": "VirtualMachine"}
			if name != "" {
				payload["name"] = name
			}
			if projectFilter != "" {
				payload["project"] = projectFilter
			}
			if allProjects {
				payload["allProjects"] = true
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
		q := db.DB
		if appFilter != "" {
			q = q.Where("application = ?", appFilter)
		}
		if projectFilter != "" {
			var appNames []string
			if err := db.DB.Model(&v1.Application{}).Where("metadata_project = ?", projectFilter).Pluck("name", &appNames).Error; err != nil {
				http.Error(w, "failed to filter virtualmachines by project", http.StatusInternalServerError)
				return
			}
			if len(appNames) > 0 {
				q = q.Where("application IN ? OR ((application IS NULL OR application = '') AND metadata_project = ?)", appNames, projectFilter)
			} else {
				q = q.Where("(application IS NULL OR application = '') AND metadata_project = ?", projectFilter)
			}
		} else if !allProjects {
			var appNames []string
			if err := db.DB.Model(&v1.Application{}).Where("metadata_project IS NULL OR metadata_project = ''").Pluck("name", &appNames).Error; err != nil {
				http.Error(w, "failed to filter virtualmachines by default project scope", http.StatusInternalServerError)
				return
			}
			if len(appNames) > 0 {
				q = q.Where("application IN ? OR ((application IS NULL OR application = '') AND (metadata_project IS NULL OR metadata_project = ''))", appNames)
			} else {
				q = q.Where("(application IS NULL OR application = '') AND (metadata_project IS NULL OR metadata_project = '')")
			}
		}
		if err := q.Find(&vms).Error; err != nil {
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
		payload := map[string]interface{}{"kind": kind}
		if name != "" {
			payload["name"] = name
		}
		if projectFilter != "" {
			payload["project"] = projectFilter
		}
		if allProjects {
			payload["allProjects"] = true
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

	case "Project":
		payload := map[string]interface{}{"kind": kind}
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
	projectFilter := r.URL.Query().Get("project")
	allProjects := r.URL.Query().Get("all-projects") == "true"
	switch kind {
	case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData":
		payload := map[string]interface{}{"kind": kind, "name": name}
		if projectFilter != "" {
			payload["project"] = projectFilter
		}
		if allProjects {
			payload["allProjects"] = true
		}
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
		if r.URL.Query().Get("all-vms") == "true" {
			payload := map[string]interface{}{"kind": kind}
			if name != "" {
				payload["name"] = name
			}
			if projectFilter != "" {
				payload["project"] = projectFilter
			}
			if allProjects {
				payload["allProjects"] = true
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
	case "Project":
		payload := map[string]interface{}{"kind": kind, "name": name}
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
		// Mark the application as removing and also mark related components
		// and VMs so the reconciler can perform ordered deletion.
		app.Status.ObservedState = "Removing"
		app.Status.Ready = false
		app.Status.LastChecked = time.Now()

		// Mark VMs referencing this application as Removing
		var appVMs []v1.VirtualMachine
		if err := db.DB.Where("application = ?", app.Metadata.Name).Find(&appVMs).Error; err == nil {
			for _, vm := range appVMs {
				vm.Status.ObservedState = "Removing"
				vm.Status.Ready = false
				vm.Status.LastChecked = time.Now()
				db.DB.Save(&vm)
			}
		}

		// Mark components referenced by this application as Removing
		var compNames []string
		for _, cref := range app.Spec.Components {
			compNames = append(compNames, cref.Name)
		}
		if len(compNames) > 0 {
			var comps []v1.Component
			if err := db.DB.Where("name IN ?", compNames).Find(&comps).Error; err == nil {
				for _, comp := range comps {
					comp.Status.ObservedState = "Removing"
					comp.Status.Ready = false
					comp.Status.LastChecked = time.Now()
					db.DB.Save(&comp)
				}
			}
		}

		if err := db.DB.Save(&app).Error; err != nil {
			http.Error(w, "failed to mark application for removal", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"Removing"}`))
		return
	case "Component":
		var comp v1.Component
		if db.DB == nil || db.DB.Where("name = ?", name).First(&comp).Error != nil {
			http.Error(w, "component not found", http.StatusNotFound)
			return
		}
		// Mark component as removing; reconciler will delete when VMs are gone
		comp.Status.ObservedState = "Removing"
		comp.Status.Ready = false
		comp.Status.LastChecked = time.Now()
		if err := db.DB.Save(&comp).Error; err != nil {
			http.Error(w, "failed to mark component for removal", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"Removing"}`))
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
				w.Write([]byte(`{"status":"Deleted"}`))
				return
			}
			http.Error(w, "virtualmachine not found", http.StatusNotFound)
			return
		}
		// Instead of deleting immediately, mark VM as Removing so the reconciler
		// will perform deletion in the proper order (VMs -> Components -> Applications).
		vm.Status.ObservedState = "Removing"
		vm.Status.Ready = false
		vm.Status.LastChecked = time.Now()
		if err := db.DB.Save(&vm).Error; err != nil {
			http.Error(w, "failed to mark virtualmachine for removal", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"Removing"}`))
		return
	case "Network", "Volume", "SSHKey", "SecurityGroup", "AffinityGroup", "UserData", "Template", "Snapshot", "Project":
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
	// Accept project in either metadata.project or spec.project and keep them in sync.
	appProject := strings.TrimSpace(app.Metadata.Project)
	if appProject == "" {
		appProject = strings.TrimSpace(app.Spec.Project)
	}
	if appProject != "" {
		app.Metadata.Project = appProject
		app.Spec.Project = appProject
	}

	// Upsert: look up an existing record by name so Save() performs an UPDATE
	// rather than an INSERT (Save with ID=0 always inserts a new row).
	var existing v1.Application
	if err := db.DB.Where("name = ?", app.Metadata.Name).First(&existing).Error; err == nil {
		app.Model = existing.Model
	}

	// Save desired state to database and mark as Starting.
	if app.Status.ObservedState == "" {
		app.Status.ObservedState = "Starting"
		app.Status.Ready = false
		app.Status.LastChecked = time.Now()
	}
	if err := db.DB.Save(app).Error; err != nil {
		return err
	}

	// Ensure Component records exist and are linked to this application.
	if err := ensureComponentsForApplication(app); err != nil {
		return err
	}

	// Actual creation of component VMs is handled by the reconciler.
	return nil
}

// ensureComponentsForApplication creates Component DB records for any components
// referenced by the Application if they do not already exist. It also ensures
// the Component.Application field is set to the owning application name.
func ensureComponentsForApplication(app *v1.Application) error {
	appProject := strings.TrimSpace(app.Metadata.Project)
	if appProject == "" {
		appProject = strings.TrimSpace(app.Spec.Project)
	}

	for _, cref := range app.Spec.Components {
		var comp v1.Component
		if err := db.DB.Where("name = ?", cref.Name).First(&comp).Error; err != nil {
			// component not found: create a new record
			comp = v1.Component{
				APIVersion: v1.APIVersion,
				Kind:       "Component",
				Metadata:   v1.Metadata{Name: cref.Name, Project: appProject},
				Spec: v1.ComponentSpec{
					VirtualMachineSpec: cref.VirtualMachineSpec,
					Replicas:           cref.Replicas,
					MinHealthy:         cref.MinHealthy,
					HealthChecks:       cref.HealthChecks,
					Overrides:          cref.Overrides,
				},
				Status:      v1.Status{ObservedState: "Starting", Ready: false, LastChecked: time.Now()},
				Application: app.Metadata.Name,
			}
			if err := db.DB.Create(&comp).Error; err != nil {
				return err
			}
		} else {
			log.Println("Component already exists, ensuring application ownership is correct:", comp.Application)
			// If the existing component is already owned by a different
			// application, fail rather than overwrite ownership.
			if comp.Application != "" && comp.Application != app.Metadata.Name {
				return fmt.Errorf("component %s already owned by application %s", comp.Metadata.Name, comp.Application)
			}
			// If no owner recorded, set the application owner.
			if comp.Application == "" {
				log.Printf("Linking existing component %s to application %s", comp.Metadata.Name, app.Metadata.Name)
				// Perform a targeted update to avoid overwriting other fields on the existing record.
				if err := db.DB.Model(&v1.Component{}).Where("name = ?", comp.Metadata.Name).Update("application", app.Metadata.Name).Error; err != nil {
					log.Printf("failed to link component %s to application %s: %v", comp.Metadata.Name, app.Metadata.Name, err)
					return err
				}
				log.Printf("linked component %s -> application %s", comp.Metadata.Name, app.Metadata.Name)
				// refresh local copy and mark Starting so reconciler will create VMs
				comp.Application = app.Metadata.Name
				comp.Status.ObservedState = "Starting"
				comp.Status.Ready = false
				comp.Status.LastChecked = time.Now()
				if err := db.DB.Save(&comp).Error; err != nil {
					log.Printf("failed to persist updated status for component %s: %v", comp.Metadata.Name, err)
					return err
				}
			}

			if appProject != "" && comp.Metadata.Project != appProject {
				if err := db.DB.Model(&v1.Component{}).Where("name = ?", comp.Metadata.Name).Update("metadata_project", appProject).Error; err != nil {
					log.Printf("failed to set component %s project to %s: %v", comp.Metadata.Name, appProject, err)
					return err
				}
				comp.Metadata.Project = appProject
			}

			// Also ensure existing VM records for this component are linked
			// to the owning application if they don't already have an owner.
			var vms []v1.VirtualMachine
			if err := db.DB.Where("component = ?", comp.Metadata.Name).Find(&vms).Error; err == nil {
				for _, vm := range vms {
					changed := false
					if vm.Application == "" {
						log.Printf("Linking VM %s to application %s", vm.Metadata.Name, app.Metadata.Name)
						vm.Application = app.Metadata.Name
						changed = true
					}
					if appProject != "" {
						if vm.Metadata.Project != appProject {
							vm.Metadata.Project = appProject
							changed = true
						}
						if vm.Spec.Project != appProject {
							vm.Spec.Project = appProject
							changed = true
						}
					}
					if changed {
						if err := db.DB.Save(&vm).Error; err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

// applyComponent creates/updates a Component resource
func (c *Controller) applyComponent(comp *v1.Component) error {
	if comp.Status.ObservedState == "" {
		comp.Status.ObservedState = "Created"
		comp.Status.Ready = false
		comp.Status.LastChecked = time.Now()
	}
	// Resolve base VM spec (by reference or inline) and compute effective spec
	var base v1.VirtualMachineSpec
	if comp.Spec.VirtualMachineSpec != "" {
		var vsr v1.VirtualMachineSpecResource
		if err := db.DB.Where("name = ?", comp.Spec.VirtualMachineSpec).First(&vsr).Error; err == nil {
			base = vsr.Spec
		}
	} else {
		// if no referenced spec, allow inline EffectiveSpec to be used as base
		base = comp.EffectiveSpec
	}

	effective := mergeVMSpec(base, comp.Spec.Overrides)
	comp.EffectiveSpec = effective

	// Ensure we don't overwrite a component's existing application ownership
	var existing v1.Component
	if err := db.DB.Where("name = ?", comp.Metadata.Name).First(&existing).Error; err == nil {
		if existing.Application != "" && comp.Application != "" && existing.Application != comp.Application {
			return fmt.Errorf("component %s already owned by application %s", comp.Metadata.Name, existing.Application)
		}
		// preserve existing ownership if caller didn't set it
		if existing.Application != "" && comp.Application == "" {
			comp.Application = existing.Application
		}
		// Use the existing record's primary key so Save() performs UPDATE
		comp.Model = existing.Model
	}

	// Persist desired component with effective spec
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
	if vs.Status.ObservedState == "" {
		vs.Status.ObservedState = "Created"
		vs.Status.Ready = false
		vs.Status.LastChecked = time.Now()
	}
	return db.DB.Create(vs).Error
}

// applyVM creates/updates a VirtualMachine resource
func (c *Controller) applyVM(vm *v1.VirtualMachine) error {
	// Attempt to find the VM in CloudStack by CloudStackID or by name
	// If CloudStackID is empty, try to discover VM in CloudStack
	if vm.CloudStackID == "" {
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
			vm.CloudStackID = resp.VirtualMachines[0].Id
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
	if vm.CloudStackID != "" {
		existing.CloudStackID = vm.CloudStackID
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
