package controller_test

import (
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/controller"
)

func TestLoadCloudStackClientFromK8s_NotInCluster(t *testing.T) {
	_, err := controller.LoadCloudStackClientFromK8s()
	if err == nil {
		t.Fatal("expected error when not running in-cluster")
	}
}

func TestDetectDrift_EmptyCloudStackIDAndReconcileDrift(t *testing.T) {
	c := controller.New(nil)
	vm := &v1.VirtualMachine{
		Metadata:     v1.Metadata{Name: "vm-no-id"},
		CloudStackID: "",
		Status:       v1.Status{ObservedState: "Running", Drift: true},
	}

	if err := c.DetectDrift(vm); err != nil {
		t.Fatalf("DetectDrift returned error: %v", err)
	}
	if vm.Status.Drift {
		t.Fatal("expected DetectDrift to clear drift when CloudStackID is empty")
	}

	// ReconcileDrift should be a no-op when drift is false.
	if err := c.ReconcileDrift(vm); err != nil {
		t.Fatalf("ReconcileDrift(false) returned error: %v", err)
	}

	// Current implementation also returns nil for drift=true path.
	vm.Status.Drift = true
	if err := c.ReconcileDrift(vm); err != nil {
		t.Fatalf("ReconcileDrift(true) returned error: %v", err)
	}
}
