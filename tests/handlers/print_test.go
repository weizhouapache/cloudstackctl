package handlers_test

import (
	"bytes"
	"os"
	"testing"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/handlers"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe create failed: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String()
}

func TestPrintCloudStackResource_BranchesAndFallback(t *testing.T) {
	cases := []struct {
		kind string
		obj  any
	}{
		{kind: "Volume", obj: &cs.ListVolumesResponse{Volumes: []*cs.Volume{{Name: "vol-a", Id: "v-1"}}}},
		{kind: "Network", obj: &cs.ListNetworksResponse{Networks: []*cs.Network{{Name: "net-a", Id: "n-1", Zoneid: "z-1", Type: "Shared", State: "Allocated"}}}},
		{kind: "VirtualMachine", obj: &cs.ListVirtualMachinesResponse{VirtualMachines: []*cs.VirtualMachine{{Name: "vm-a", Id: "vm-1", State: "Running"}}}},
		{kind: "Template", obj: &cs.ListTemplatesResponse{Templates: []*cs.Template{{Name: "tpl-a", Id: "t-1"}}}},
		{kind: "SSHKey", obj: &cs.ListSSHKeyPairsResponse{SSHKeyPairs: []*cs.SSHKeyPair{{Name: "k-a", Fingerprint: "fp"}}}},
		{kind: "SecurityGroup", obj: &cs.ListSecurityGroupsResponse{SecurityGroups: []*cs.SecurityGroup{{Name: "sg-a", Id: "sg-1"}}}},
		{kind: "AffinityGroup", obj: &cs.ListAffinityGroupsResponse{AffinityGroups: []*cs.AffinityGroup{{Name: "ag-a", Id: "ag-1"}}}},
		{kind: "UserData", obj: &cs.ListUserDataResponse{UserData: []*cs.UserData{{Name: "ud-a", Id: "ud-1"}}}},
		{kind: "Project", obj: &cs.ListProjectsResponse{Projects: []*cs.Project{{Name: "p-a", Id: "p-1"}}}},
	}

	for _, tc := range cases {
		out := captureStdout(t, func() {
			if err := handlers.PrintCloudStackResource(tc.kind, tc.obj); err != nil {
				t.Fatalf("PrintCloudStackResource(%s) error: %v", tc.kind, err)
			}
		})
		if out == "" {
			t.Fatalf("expected output for kind %s", tc.kind)
		}
	}

	fallbackOut := captureStdout(t, func() {
		if err := handlers.PrintCloudStackResource("UnknownKind", map[string]string{"k": "v"}); err != nil {
			t.Fatalf("fallback print error: %v", err)
		}
	})
	if fallbackOut == "" {
		t.Fatal("expected fallback JSON output")
	}
}

func TestPrintControllerTables(t *testing.T) {
	vms := []v1.VirtualMachine{{Metadata: v1.Metadata{Name: "vm-a"}, Status: v1.Status{ObservedState: "Running", Ready: true}}}
	comps := []v1.Component{{Metadata: v1.Metadata{Name: "comp-a"}, Spec: v1.ComponentSpec{Replicas: 1, VirtualMachineSpec: "spec-a"}}}
	specs := []v1.VirtualMachineSpecResource{{Metadata: v1.Metadata{Name: "spec-a"}, Spec: v1.VirtualMachineSpec{Template: "tpl-a", ServiceOffering: "small"}}}
	apps := []v1.Application{{Metadata: v1.Metadata{Name: "app-a"}, Spec: v1.ApplicationSpec{Components: []v1.ComponentRef{{Name: "comp-a"}}}, Status: v1.Status{ObservedState: "Healthy", Ready: true}}}

	if out := captureStdout(t, func() { handlers.PrintVMsFromController(vms) }); out == "" {
		t.Fatal("expected PrintVMsFromController output")
	}
	if out := captureStdout(t, func() { handlers.PrintComponents(comps) }); out == "" {
		t.Fatal("expected PrintComponents output")
	}
	if out := captureStdout(t, func() { handlers.PrintVMSpecs(specs) }); out == "" {
		t.Fatal("expected PrintVMSpecs output")
	}
	if out := captureStdout(t, func() { handlers.PrintApplications(apps) }); out == "" {
		t.Fatal("expected PrintApplications output")
	}
}
