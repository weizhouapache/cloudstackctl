package handlers_test

import (
	"path/filepath"
	"testing"

	v1 "cloudstackctl/apis/v1"
	cloudstackpkg "cloudstackctl/pkg/cloudstack"
	"cloudstackctl/pkg/handlers"
)

func withMissingCloudStackCreds(t *testing.T) {
	t.Helper()
	t.Setenv("CLOUDSTACK_ENDPOINT", "")
	t.Setenv("CLOUDSTACK_API_KEY", "")
	t.Setenv("CLOUDSTACK_SECRET_KEY", "")
	t.Setenv("VERIFY_SSL", "")
	cloudstackpkg.SetConfigFile(filepath.Join(t.TempDir(), "no-file.env"))
	t.Cleanup(func() { cloudstackpkg.SetConfigFile("") })
}

func expectError(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error from %s", name)
	}
}

func TestCloudStackHandlerErrorPaths_MissingClientConfig(t *testing.T) {
	withMissingCloudStackCreds(t)

	if _, err := handlers.ListAffinityGroups("", "", false); err == nil {
		t.Fatal("expected ListAffinityGroups error")
	}
	if _, err := handlers.ListNetworks("", "", false); err == nil {
		t.Fatal("expected ListNetworks error")
	}
	if _, err := handlers.ListProjects(""); err == nil {
		t.Fatal("expected ListProjects error")
	}
	if _, err := handlers.ListSecurityGroups("", "", false); err == nil {
		t.Fatal("expected ListSecurityGroups error")
	}
	if err := handlers.ListSnapshots(""); err == nil {
		t.Fatal("expected ListSnapshots error")
	}
	if _, err := handlers.ListSSHKeys("", "", false); err == nil {
		t.Fatal("expected ListSSHKeys error")
	}
	if _, err := handlers.ListTemplates("", "", false); err == nil {
		t.Fatal("expected ListTemplates error")
	}
	if _, err := handlers.ListUserData("", "", false); err == nil {
		t.Fatal("expected ListUserData error")
	}
	if _, err := handlers.ListVMs("", "", false); err == nil {
		t.Fatal("expected ListVMs error")
	}
	if _, err := handlers.ListVolumes("", "", false); err == nil {
		t.Fatal("expected ListVolumes error")
	}

	if _, err := handlers.ApplyAffinityGroup(&v1.AffinityGroup{Metadata: v1.Metadata{Name: "ag-a"}, Spec: v1.AffinitySpec{Type: "host anti-affinity"}}); err == nil {
		t.Fatal("expected ApplyAffinityGroup error")
	}
	if _, err := handlers.ApplyNetwork(&v1.Network{Metadata: v1.Metadata{Name: "net-a"}}); err == nil {
		t.Fatal("expected ApplyNetwork error")
	}
	if _, err := handlers.ApplyProject("p-a", "project a"); err == nil {
		t.Fatal("expected ApplyProject error")
	}
	if _, err := handlers.ApplySecurityGroup(&v1.SecurityGroup{Metadata: v1.Metadata{Name: "sg-a"}}); err == nil {
		t.Fatal("expected ApplySecurityGroup error")
	}
	if _, err := handlers.ApplySSHKey(&v1.SSHKey{Metadata: v1.Metadata{Name: "k-a"}}); err == nil {
		t.Fatal("expected ApplySSHKey error")
	}
	if _, err := handlers.ApplyUserData(&v1.UserData{Metadata: v1.Metadata{Name: "ud-a"}}); err == nil {
		t.Fatal("expected ApplyUserData error")
	}
	if _, err := handlers.ApplyVirtualMachine(&v1.VirtualMachine{Metadata: v1.Metadata{Name: "vm-a"}}); err == nil {
		t.Fatal("expected ApplyVirtualMachine error")
	}
	if _, err := handlers.ApplyVolume(&v1.Volume{Metadata: v1.Metadata{Name: "vol-a"}}); err == nil {
		t.Fatal("expected ApplyVolume error")
	}
}
