package controller

import (
	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/db"
	"encoding/json"
	"log"
)

// DescribeApplication prints a single Application by name
func DescribeApplication(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}

	var app v1.Application
	if err := db.DB.Where("name = ?", name).First(&app).Error; err != nil {
		log.Fatalf("Application %s not found: %v", name, err)
	}
	data, _ := json.MarshalIndent(app, "", "  ")
	log.Println(string(data))
}

// DescribeComponent prints a single Component by name
func DescribeComponent(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}

	var comp v1.Component
	if err := db.DB.Where("name = ?", name).First(&comp).Error; err != nil {
		log.Fatalf("Component %s not found: %v", name, err)
	}
	data, _ := json.MarshalIndent(comp, "", "  ")
	log.Println(string(data))
}

// DescribeVM prints a single VirtualMachine by name (from DB)
func DescribeVM(name string) {
	if db.DB == nil {
		if err := db.Init(); err != nil {
			log.Fatalf("Database unavailable: %v", err)
		}
	}

	var vm v1.VirtualMachine
	if err := db.DB.Where("name = ?", name).First(&vm).Error; err != nil {
		log.Fatalf("VM %s not found: %v", name, err)
	}
	data, _ := json.MarshalIndent(vm, "", "  ")
	log.Println(string(data))
}
