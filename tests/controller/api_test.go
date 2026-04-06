package controller_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/controller"
	"cloudstackctl/db"
)

func seedListScopeData(t *testing.T) {
	t.Helper()
	apps := []v1.Application{
		{Metadata: v1.Metadata{Name: "app-public", Project: ""}},
		{Metadata: v1.Metadata{Name: "app-proj", Project: "proj-1"}},
	}
	for i := range apps {
		if err := db.DB.Create(&apps[i]).Error; err != nil {
			t.Fatalf("failed to seed app %s: %v", apps[i].Metadata.Name, err)
		}
	}

	comps := []v1.Component{
		{Metadata: v1.Metadata{Name: "comp-public-app", Project: ""}, Application: "app-public"},
		{Metadata: v1.Metadata{Name: "comp-proj-app", Project: "proj-1"}, Application: "app-proj"},
		{Metadata: v1.Metadata{Name: "comp-public-standalone", Project: ""}},
		{Metadata: v1.Metadata{Name: "comp-proj-standalone", Project: "proj-1"}},
	}
	for i := range comps {
		if err := db.DB.Create(&comps[i]).Error; err != nil {
			t.Fatalf("failed to seed component %s: %v", comps[i].Metadata.Name, err)
		}
	}

	vms := []v1.VirtualMachine{
		{Metadata: v1.Metadata{Name: "vm-public-app", Project: ""}, Application: "app-public", Component: "comp-public-app"},
		{Metadata: v1.Metadata{Name: "vm-proj-app", Project: "proj-1"}, Spec: v1.VirtualMachineSpec{Project: "proj-1"}, Application: "app-proj", Component: "comp-proj-app"},
		{Metadata: v1.Metadata{Name: "vm-public-standalone", Project: ""}},
		{Metadata: v1.Metadata{Name: "vm-proj-standalone", Project: "proj-1"}, Spec: v1.VirtualMachineSpec{Project: "proj-1"}},
	}
	for i := range vms {
		if err := db.DB.Create(&vms[i]).Error; err != nil {
			t.Fatalf("failed to seed vm %s: %v", vms[i].Metadata.Name, err)
		}
	}
}

func TestControllerHandler_ApplyStatusDescribeDeleteAndList(t *testing.T) {
	setupTestDB(t)
	c := controller.New(nil)
	h := c.Handler()

	payload := strings.Join([]string{
		"apiVersion: cloudstackctl/v1\nkind: VirtualMachineSpec\nmetadata:\n  name: web-spec\nspec:\n  zone: zone-1\n  template: tpl-1\n  serviceOffering: small\n",
		"apiVersion: cloudstackctl/v1\nkind: Application\nmetadata:\n  name: app-http\n  project: proj-http\nspec:\n  components:\n    - name: web\n      virtualMachineSpec: web-spec\n      replicas: 1\n",
	}, "---\n")
	applyReq := httptest.NewRequest("POST", "/apply", strings.NewReader(payload))
	applyRR := httptest.NewRecorder()
	h.ServeHTTP(applyRR, applyReq)
	if applyRR.Code != 200 {
		t.Fatalf("apply status=%d body=%s", applyRR.Code, applyRR.Body.String())
	}

	statusReq := httptest.NewRequest("GET", "/status?kind=Application&name=app-http", nil)
	statusRR := httptest.NewRecorder()
	h.ServeHTTP(statusRR, statusReq)
	if statusRR.Code != 200 {
		t.Fatalf("status code=%d", statusRR.Code)
	}
	var app v1.Application
	if err := json.Unmarshal(statusRR.Body.Bytes(), &app); err != nil {
		t.Fatalf("decode status app: %v", err)
	}
	if app.Metadata.Project != "proj-http" || app.Spec.Project != "proj-http" {
		t.Fatalf("project sync failed: metadata=%q spec=%q", app.Metadata.Project, app.Spec.Project)
	}

	describeReq := httptest.NewRequest("GET", "/describe?kind=Application&name=app-http", nil)
	describeRR := httptest.NewRecorder()
	h.ServeHTTP(describeRR, describeReq)
	if describeRR.Code != 200 {
		t.Fatalf("describe code=%d", describeRR.Code)
	}

	listReq := httptest.NewRequest("GET", "/list?kind=Application&project=proj-http", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != 200 {
		t.Fatalf("list code=%d", listRR.Code)
	}
	var apps []v1.Application
	if err := json.Unmarshal(listRR.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode list apps: %v", err)
	}
	if len(apps) != 1 || apps[0].Metadata.Name != "app-http" {
		t.Fatalf("unexpected list apps result: %#v", apps)
	}

	deleteReq := httptest.NewRequest("POST", "/delete", strings.NewReader(`{"kind":"Application","name":"app-http"}`))
	deleteRR := httptest.NewRecorder()
	h.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != 200 {
		t.Fatalf("delete code=%d body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	if err := db.DB.Where("name = ?", "app-http").First(&app).Error; err != nil {
		t.Fatalf("reload app after delete: %v", err)
	}
	if app.Status.ObservedState != "Removing" {
		t.Fatalf("expected app Removing, got %q", app.Status.ObservedState)
	}
}

func TestControllerHandler_DefaultVsAllProjectsList(t *testing.T) {
	setupTestDB(t)
	seedListScopeData(t)
	c := controller.New(nil)
	h := c.Handler()

	defaultReq := httptest.NewRequest("GET", "/list?kind=VirtualMachine", nil)
	defaultRR := httptest.NewRecorder()
	h.ServeHTTP(defaultRR, defaultReq)
	if defaultRR.Code != 200 {
		t.Fatalf("default list code=%d", defaultRR.Code)
	}
	var defaultVMs []v1.VirtualMachine
	if err := json.Unmarshal(defaultRR.Body.Bytes(), &defaultVMs); err != nil {
		t.Fatalf("decode default vms: %v", err)
	}
	if len(defaultVMs) != 2 {
		t.Fatalf("expected 2 default vms, got %d", len(defaultVMs))
	}

	allReq := httptest.NewRequest("GET", "/list?kind=VirtualMachine&all-projects=true", nil)
	allRR := httptest.NewRecorder()
	h.ServeHTTP(allRR, allReq)
	if allRR.Code != 200 {
		t.Fatalf("all-projects list code=%d", allRR.Code)
	}
	var allVMs []v1.VirtualMachine
	if err := json.Unmarshal(allRR.Body.Bytes(), &allVMs); err != nil {
		t.Fatalf("decode all-projects vms: %v", err)
	}
	if len(allVMs) != 4 {
		t.Fatalf("expected 4 all-project vms, got %d", len(allVMs))
	}
}
