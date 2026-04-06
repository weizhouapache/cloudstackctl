package controller_test

import (
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/controller"
	"cloudstackctl/db"
)

func seedCmdPathData(t *testing.T) {
	t.Helper()
	app := v1.Application{
		Metadata: v1.Metadata{Name: "cmd-app"},
		Spec: v1.ApplicationSpec{Project: "proj-cmd", Components: []v1.ComponentRef{
			{Name: "cmd-comp"},
		}},
		Status: v1.Status{ObservedState: "Healthy", Ready: true},
	}
	comp := v1.Component{
		Metadata:    v1.Metadata{Name: "cmd-comp"},
		Application: "cmd-app",
		Spec:        v1.ComponentSpec{Replicas: 1, VirtualMachineSpec: "cmd-spec"},
		Status:      v1.Status{ObservedState: "Healthy", Ready: true},
	}
	vs := v1.VirtualMachineSpecResource{
		Metadata: v1.Metadata{Name: "cmd-spec"},
		Spec: v1.VirtualMachineSpec{
			Template:        "tpl-cmd",
			ServiceOffering: "small",
			Networks:        []string{"net-a"},
		},
	}
	vm := v1.VirtualMachine{
		Metadata:    v1.Metadata{Name: "cmd-vm"},
		Application: "cmd-app",
		Component:   "cmd-comp",
		Spec:        v1.VirtualMachineSpec{Template: "tpl-cmd", ServiceOffering: "small"},
		Status:      v1.Status{ObservedState: "Running", Ready: true},
	}
	for _, obj := range []any{&vs, &app, &comp, &vm} {
		if err := db.DB.Create(obj).Error; err != nil {
			t.Fatalf("failed to seed cmd path data: %v", err)
		}
	}
}

func TestControllerCmdGetAndDescribePaths(t *testing.T) {
	setupTestDB(t)
	seedCmdPathData(t)

	// List paths
	controller.ListApplications("")
	controller.ListComponents("")
	controller.ListVirtualMachineSpec("")
	controller.ListVMs("")

	// Single-resource paths
	controller.ListApplications("cmd-app")
	controller.ListComponents("cmd-comp")
	controller.ListVirtualMachineSpec("cmd-spec")
	controller.ListVMs("cmd-vm")

	// Describe paths
	controller.DescribeApplication("cmd-app")
	controller.DescribeComponent("cmd-comp")
	controller.DescribeVM("cmd-vm")
}

func TestControllerCmdDeletePaths_DBOnly(t *testing.T) {
	setupTestDB(t)
	seedCmdPathData(t)

	// DeleteVM path (DB-backed VM with empty CloudStackID avoids external calls)
	controller.DeleteVM("cmd-vm")
	var vmCount int64
	if err := db.DB.Model(&v1.VirtualMachine{}).Where("name = ?", "cmd-vm").Count(&vmCount).Error; err != nil {
		t.Fatalf("count cmd-vm failed: %v", err)
	}
	if vmCount != 0 {
		t.Fatalf("expected cmd-vm deleted, count=%d", vmCount)
	}

	// DeleteComponent path (not referenced by application after removing app ref)
	if err := db.DB.Model(&v1.Application{}).Where("name = ?", "cmd-app").Update("components", "[]").Error; err != nil {
		t.Fatalf("failed clearing app component refs: %v", err)
	}
	controller.DeleteComponent("cmd-comp")
	var compCount int64
	if err := db.DB.Model(&v1.Component{}).Where("name = ?", "cmd-comp").Count(&compCount).Error; err != nil {
		t.Fatalf("count cmd-comp failed: %v", err)
	}
	if compCount != 0 {
		t.Fatalf("expected cmd-comp deleted, count=%d", compCount)
	}

	// Re-seed app + component to validate DeleteApplication deletes both
	app := v1.Application{Metadata: v1.Metadata{Name: "cmd-app-2"}, Spec: v1.ApplicationSpec{Components: []v1.ComponentRef{{Name: "cmd-comp-2"}}}}
	comp := v1.Component{Metadata: v1.Metadata{Name: "cmd-comp-2"}, Application: "cmd-app-2"}
	if err := db.DB.Create(&app).Error; err != nil {
		t.Fatalf("seed cmd-app-2 failed: %v", err)
	}
	if err := db.DB.Create(&comp).Error; err != nil {
		t.Fatalf("seed cmd-comp-2 failed: %v", err)
	}
	controller.DeleteApplication("cmd-app-2")

	var appCount int64
	if err := db.DB.Model(&v1.Application{}).Where("name = ?", "cmd-app-2").Count(&appCount).Error; err != nil {
		t.Fatalf("count cmd-app-2 failed: %v", err)
	}
	if appCount != 0 {
		t.Fatalf("expected cmd-app-2 deleted, count=%d", appCount)
	}
	if err := db.DB.Model(&v1.Component{}).Where("name = ?", "cmd-comp-2").Count(&compCount).Error; err != nil {
		t.Fatalf("count cmd-comp-2 failed: %v", err)
	}
	if compCount != 0 {
		t.Fatalf("expected cmd-comp-2 deleted, count=%d", compCount)
	}
}
