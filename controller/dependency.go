package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"fmt"
	"log"
	"time"
)

// ResolveComponentDependencies enforces component creation order:
// Only create next component when previous component is healthy (Ready=true)
func (c *Controller) ResolveComponentDependencies(app *v1.Application) error {
	log.Printf("Resolving dependencies for application: %s", app.Metadata.Name)

	// Process components in order (dependency enforcement)
	for i, compRef := range app.Spec.Components {
		var component v1.Component
		if err := db.DB.Where("name = ?", compRef.Name).First(&component).Error; err != nil {
			log.Printf("Component %s not found: %v", compRef.Name, err)
			return err
		}

		// Wait for component to be healthy (Ready=true) before next component
		if i > 0 {
			prevCompRef := app.Spec.Components[i-1]
			if err := c.waitForComponentHealth(prevCompRef.Name); err != nil {
				log.Printf("Previous component %s not healthy: %v", prevCompRef.Name, err)
				return err
			}
		}

		// Create component VMs
		log.Printf("Creating component %s (replicas: %d)", compRef.Name, compRef.Replicas)
		if err := c.createComponentVMs(&component, compRef); err != nil {
			return err
		}
	}

	// Mark application as ready
	app.Status.Ready = true
	app.Status.ObservedState = "Running"
	app.Status.LastChecked = time.Now()
	return db.DB.Save(app).Error
}

// waitForComponentHealth blocks until component is healthy (Ready=true)
func (c *Controller) waitForComponentHealth(componentName string) error {
	log.Printf("Waiting for component %s to become healthy", componentName)

	timeout := time.After(5 * time.Minute)
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
