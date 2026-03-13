package main

import (
	"log"
	"os"

	"cloudstackctl/controller"
	"cloudstackctl/db"
	"cloudstackctl/pkg/cloudstack"
)

func main() {
	// Initialize database
	if err := db.Init(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create CloudStack client (env/file-based)
	csClient, err := cloudstack.NewClient()
	if err != nil {
		log.Fatalf("Failed to initialize CloudStack client: %v", err)
	}

	// Prefer loading credentials directly from a Kubernetes Secret when running in-cluster.
	ctrlClient := csClient
	if k8sClient, err := controller.LoadCloudStackClientFromK8s(); err == nil {
		ctrlClient = k8sClient
	} else {
		log.Println("Kubernetes Secret not available or not running in-cluster; using environment/file-based credentials for controller")
	}

	// Ensure logs are written to a file instead of stdout
	logFile := os.Getenv("CONTROLLER_LOG_FILE")
	if logFile == "" {
		logFile = "/var/log/cloudstackctl-controller.log"
	}
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(f)
		defer f.Close()
	} else {
		log.Printf("Warning: could not open log file %s: %v; falling back to stdout", logFile, err)
	}

	// Create controller and start reconciliation loop (blocks)
	ctrl := controller.New(ctrlClient)
	ctrl.Start()
}
