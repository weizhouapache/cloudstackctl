package controller_test

import (
	"strings"
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/controller"
	"cloudstackctl/db"
)

func TestControllerApply_ComponentComputesEffectiveSpec(t *testing.T) {
	setupTestDB(t)
	c := controller.New(nil)

	vs := &v1.VirtualMachineSpecResource{
		Metadata: v1.Metadata{Name: "base-spec"},
		Spec: v1.VirtualMachineSpec{
			Template:        "tpl-base",
			ServiceOffering: "so-base",
			SSHKeys:         []string{"key-a"},
			UserDataRefs:    []string{"ud-a"},
		},
	}
	if err := c.Apply(vs); err != nil {
		t.Fatalf("apply vmspec failed: %v", err)
	}

	comp := &v1.Component{
		Metadata: v1.Metadata{Name: "comp-a"},
		Spec: v1.ComponentSpec{
			VirtualMachineSpec: "base-spec",
			Overrides: v1.ComponentOverrides{
				Template:        "tpl-new",
				ServiceOffering: "so-new",
				SSHKeys:         []string{"key-b"},
				UserDataRefs:    []string{"ud-b"},
			},
		},
	}
	if err := c.Apply(comp); err != nil {
		t.Fatalf("apply component failed: %v", err)
	}
	if comp.EffectiveSpec.Template != "tpl-new" || comp.EffectiveSpec.ServiceOffering != "so-new" {
		t.Fatalf("unexpected effective spec overrides: %#v", comp.EffectiveSpec)
	}
	if len(comp.EffectiveSpec.SSHKeys) != 2 || len(comp.EffectiveSpec.UserDataRefs) != 2 {
		t.Fatalf("expected merged effective spec slices, got %#v", comp.EffectiveSpec)
	}
}

func TestControllerResolveDependenciesAndHealth(t *testing.T) {
	setupTestDB(t)
	c := controller.New(nil)

	cycleApp := &v1.Application{
		Metadata: v1.Metadata{Name: "app-cycle"},
		Spec: v1.ApplicationSpec{Components: []v1.ComponentRef{
			{Name: "a", DependsOn: "b"},
			{Name: "b", DependsOn: "a"},
		}},
	}
	if err := c.ResolveComponentDependencies(cycleApp); err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}

	comp := v1.Component{Metadata: v1.Metadata{Name: "comp-start"}, Spec: v1.ComponentSpec{Replicas: 0}}
	if err := db.DB.Create(&comp).Error; err != nil {
		t.Fatalf("seed comp-start failed: %v", err)
	}
	app := &v1.Application{
		Metadata: v1.Metadata{Name: "app-start"},
		Spec: v1.ApplicationSpec{Components: []v1.ComponentRef{{Name: "comp-start", Replicas: 0}}},
	}
	if err := c.ResolveComponentDependencies(app); err != nil {
		t.Fatalf("ResolveComponentDependencies failed: %v", err)
	}
	var saved v1.Application
	if err := db.DB.Where("name = ?", "app-start").First(&saved).Error; err != nil {
		t.Fatalf("load saved app-start failed: %v", err)
	}
	if saved.Status.ObservedState != "Started" || saved.Status.Ready {
		t.Fatalf("expected Started/false, got %q/%v", saved.Status.ObservedState, saved.Status.Ready)
	}

	healthyComp := v1.Component{Metadata: v1.Metadata{Name: "healthy-zero"}, Spec: v1.ComponentSpec{Replicas: 0}}
	if err := db.DB.Create(&healthyComp).Error; err != nil {
		t.Fatalf("seed healthy comp failed: %v", err)
	}
	healthy, err := c.CheckComponentHealth(&healthyComp)
	if err != nil {
		t.Fatalf("CheckComponentHealth failed: %v", err)
	}
	if !healthy || healthyComp.Status.ObservedState != "Healthy" || !healthyComp.Status.Ready {
		t.Fatalf("expected Healthy/true, got healthy=%v state=%q ready=%v", healthy, healthyComp.Status.ObservedState, healthyComp.Status.Ready)
	}
}

func TestControllerReconcileApplication_NoComponentsHealthy(t *testing.T) {
	setupTestDB(t)
	c := controller.New(nil)

	app := v1.Application{
		Metadata: v1.Metadata{Name: "reconcile-app"},
		Status:   v1.Status{ObservedState: "Starting", Ready: false},
	}
	if err := db.DB.Create(&app).Error; err != nil {
		t.Fatalf("seed app failed: %v", err)
	}
	if err := c.ReconcileApplication(&app); err != nil {
		t.Fatalf("ReconcileApplication failed: %v", err)
	}
	if app.Status.ObservedState != "Healthy" || !app.Status.Ready {
		t.Fatalf("expected Healthy/true, got %q/%v", app.Status.ObservedState, app.Status.Ready)
	}
}
