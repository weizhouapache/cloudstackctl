package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"fmt"
	"log"
	"strings"
	"time"
)

// ResolveComponentDependencies enforces component creation order:
// Only create next component when previous component is healthy (Ready=true)
func (c *Controller) ResolveComponentDependencies(app *v1.Application) error {
	log.Printf("Resolving dependencies for application: %s", app.Metadata.Name)

	// Build mapping of component refs by name
	refs := map[string]v1.ComponentRef{}
	nodes := map[string]struct{}{}
	for _, cr := range app.Spec.Components {
		refs[cr.Name] = cr
		nodes[cr.Name] = struct{}{}
	}

	// Build adjacency and indegree for topological sort
	adj := map[string][]string{}
	indegree := map[string]int{}
	for name := range nodes {
		indegree[name] = 0
	}
	for _, cr := range app.Spec.Components {
		if cr.DependsOn == "" {
			continue
		}
		deps := strings.Split(cr.DependsOn, ",")
		for _, d := range deps {
			dep := strings.TrimSpace(d)
			if dep == "" {
				continue
			}
			// Only add edges for components that are part of this application
			if _, ok := nodes[dep]; ok {
				adj[dep] = append(adj[dep], cr.Name)
				indegree[cr.Name]++
			}
		}
	}

	// Kahn's algorithm
	var queue []string
	for name, deg := range indegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	var order []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, nb := range adj[n] {
			indegree[nb]--
			if indegree[nb] == 0 {
				queue = append(queue, nb)
			}
		}
	}

	// If we couldn't sort all nodes, there is a cycle
	if len(order) != len(nodes) {
		return logError("component dependency cycle detected for application %s", app.Metadata.Name)
	}

	// Create components in topologically-sorted order. For each component,
	// wait for its declared dependencies (even if they were outside the
	// app.Spec.Components list) to be healthy before creating.
	for _, name := range order {
		compRef := refs[name]
		var component v1.Component
		if err := db.DB.Where("name = ?", compRef.Name).First(&component).Error; err != nil {
			log.Printf("Component %s not found: %v", compRef.Name, err)
			return err
		}

		if compRef.DependsOn != "" {
			deps := strings.Split(compRef.DependsOn, ",")
			for _, d := range deps {
				depName := strings.TrimSpace(d)
				if depName == "" {
					continue
				}
				if err := c.waitForComponentHealth(depName); err != nil {
					log.Printf("Dependency component %s not healthy: %v", depName, err)
					return err
				}
			}
		}

		// If the Application's ComponentRef did not specify replicas, fall back
		// to the persisted Component.Spec.Replicas value.
		if compRef.Replicas == 0 {
			compRef.Replicas = component.Spec.Replicas
		}
		log.Printf("Creating component %s (replicas: %d)", compRef.Name, compRef.Replicas)
		if compRef.Replicas == 0 {
			log.Printf("Warning: component %s has replicas=0; skipping creation", compRef.Name)
			continue
		}
		if err := c.createComponentVMs(app.Metadata.Name, &component, compRef); err != nil {
			return err
		}
	}

	// Mark application as ready if it's not in Removing state
	if app.Status.ObservedState != "Removing" {
		app.Status.Ready = false // not healthy yet, just started
		app.Status.ObservedState = "Started"
		app.Status.LastChecked = time.Now()
		return db.DB.Save(app).Error
	}
	// If application is Removing, persist only LastChecked
	app.Status.LastChecked = time.Now()
	return db.DB.Save(app).Error
}

// waitForComponentHealth blocks until component is healthy (Ready=true)
func (c *Controller) waitForComponentHealth(componentName string) error {
	log.Printf("Waiting for component %s to become healthy", componentName)

	timeout := time.After(15 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return logError("Component %s health check timed out", componentName)
		case <-ticker.C:
			var component v1.Component
			if err := db.DB.Where("name = ?", componentName).First(&component).Error; err != nil {
				return err
			}

			// if component is in Removing state, consider it unhealthy and continue waiting without logging to avoid noise during deletion
			if component.Status.ObservedState == "Removing" {
				continue
			}

			// If not ready, attempt to actively reconcile the component so that
			// VMs are created and health checks can progress. This avoids a
			// deadlock where the caller is waiting but no reconciler runs.
			if component.Status.Ready {
				log.Printf("Component %s is healthy", componentName)
				return nil
			}
			// Trigger an immediate reconcile to drive the component towards readiness.
			if err := c.ReconcileComponent(&component); err != nil {
				log.Printf("waitForComponentHealth: reconcile attempt for %s returned error: %v", componentName, err)
				// don't return here; keep waiting until timeout to allow retries
			}
			if component.Status.Ready {
				log.Printf("Component %s is healthy", componentName)
				return nil
			}
			log.Printf("Component %s not healthy yet (state: %s)", componentName, component.Status.ObservedState)
		}
	}
}

// logError creates a formatted error with log message
func logError(format string, args ...interface{}) error {
	log.Printf(format, args...)
	return fmt.Errorf(format, args...)
}
