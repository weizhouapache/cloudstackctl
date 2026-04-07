package e2e_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	v1 "cloudstackctl/apis/v1"
	"cloudstackctl/pkg/handlers"

	cs "github.com/apache/cloudstack-go/v2/cloudstack"
)

func TestE2E_Unmanaged_ListAllKinds(t *testing.T) {
	requireUnmanagedE2EEnabled(t)

	if _, err := handlers.ListProjects(""); err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if _, err := handlers.ListNetworks("", "", true); err != nil {
		t.Fatalf("ListNetworks failed: %v", err)
	}
	if _, err := handlers.ListVolumes("", "", true); err != nil {
		t.Fatalf("ListVolumes failed: %v", err)
	}
	if _, err := handlers.ListSSHKeys("", "", true); err != nil {
		t.Fatalf("ListSSHKeys failed: %v", err)
	}
	if _, err := handlers.ListSecurityGroups("", "", true); err != nil {
		t.Fatalf("ListSecurityGroups failed: %v", err)
	}
	if _, err := handlers.ListAffinityGroups("", "", true); err != nil {
		t.Fatalf("ListAffinityGroups failed: %v", err)
	}
	if _, err := handlers.ListUserData("", "", true); err != nil {
		t.Fatalf("ListUserData failed: %v", err)
	}
	if _, err := handlers.ListTemplates("", "", true); err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}
	if _, err := handlers.ListVMs("", "", true); err != nil {
		t.Fatalf("ListVMs failed: %v", err)
	}
	if err := handlers.ListSnapshots(""); err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
}

func TestE2E_Unmanaged_DescribeAllKinds_WhenPresent(t *testing.T) {
	requireUnmanagedE2EEnabled(t)

	respAny, err := handlers.ListProjects("")
	if err != nil {
		t.Skipf("skipping unmanaged describe smoke: ListProjects failed: %v", err)
	}
	if resp, ok := respAny.(*cs.ListProjectsResponse); ok && resp != nil && len(resp.Projects) > 0 {
		if _, err := handlers.DescribeProject(resp.Projects[0].Name); err != nil {
			t.Fatalf("DescribeProject failed: %v", err)
		}
	}

	if respAny, err := handlers.ListNetworks("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListNetworksResponse); ok && resp != nil && len(resp.Networks) > 0 {
			if _, err := handlers.DescribeNetwork(resp.Networks[0].Name, "", true); err != nil {
				t.Fatalf("DescribeNetwork failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListVolumes("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListVolumesResponse); ok && resp != nil && len(resp.Volumes) > 0 {
			if _, err := handlers.DescribeVolume(resp.Volumes[0].Name, "", true); err != nil {
				t.Fatalf("DescribeVolume failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListSSHKeys("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListSSHKeyPairsResponse); ok && resp != nil && len(resp.SSHKeyPairs) > 0 {
			if _, err := handlers.DescribeSSHKey(resp.SSHKeyPairs[0].Name, "", true); err != nil {
				t.Fatalf("DescribeSSHKey failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListSecurityGroups("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListSecurityGroupsResponse); ok && resp != nil && len(resp.SecurityGroups) > 0 {
			if _, err := handlers.DescribeSecurityGroup(resp.SecurityGroups[0].Name, "", true); err != nil {
				t.Fatalf("DescribeSecurityGroup failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListAffinityGroups("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListAffinityGroupsResponse); ok && resp != nil && len(resp.AffinityGroups) > 0 {
			if _, err := handlers.DescribeAffinityGroup(resp.AffinityGroups[0].Name, "", true); err != nil {
				t.Fatalf("DescribeAffinityGroup failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListUserData("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListUserDataResponse); ok && resp != nil && len(resp.UserData) > 0 {
			if _, err := handlers.DescribeUserData(resp.UserData[0].Name, "", true); err != nil {
				t.Fatalf("DescribeUserData failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListTemplates("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListTemplatesResponse); ok && resp != nil && len(resp.Templates) > 0 {
			if _, err := handlers.DescribeTemplate(resp.Templates[0].Name, "", true); err != nil {
				t.Fatalf("DescribeTemplate failed: %v", err)
			}
		}
	}

	if respAny, err := handlers.ListVMs("", "", true); err == nil {
		if resp, ok := respAny.(*cs.ListVirtualMachinesResponse); ok && resp != nil && len(resp.VirtualMachines) > 0 {
			if _, err := handlers.DescribeVM(resp.VirtualMachines[0].Name, "", true); err != nil {
				t.Fatalf("DescribeVM failed: %v", err)
			}
		}
	}

	// Snapshots are listed with a different helper shape; do a best-effort describe if a known snapshot name is provided.
	if snapName := ""; snapName != "" {
		if _, err := handlers.DescribeSnapshot(snapName); err != nil {
			t.Fatalf("DescribeSnapshot(%s) failed: %v", snapName, err)
		}
	}
}

func TestE2E_Unmanaged_ProjectCRUD_OptionalMutation(t *testing.T) {
	requireUnmanagedE2EEnabled(t)
	if !envEnabled("E2E_ALLOW_MUTATION") {
		t.Skip("set E2E_ALLOW_MUTATION=true to run create/delete e2e tests")
	}

	name := fmt.Sprintf("csctl-e2e-%d", time.Now().UnixNano())
	display := "cloudstackctl e2e"

	id, err := handlers.ApplyProject(name, display)
	if err != nil {
		t.Fatalf("ApplyProject failed: %v", err)
	}
	if id == "" {
		t.Fatalf("ApplyProject returned empty id")
	}

	defer func() {
		_, _ = handlers.DeleteProject(name)
	}()

	obj, err := handlers.DescribeProject(name)
	if err != nil {
		t.Fatalf("DescribeProject after create failed: %v", err)
	}
	proj, ok := obj.(*cs.Project)
	if !ok {
		t.Fatalf("unexpected describe project type: %T", obj)
	}
	if proj.Name != name {
		t.Fatalf("DescribeProject name mismatch: got %s want %s", proj.Name, name)
	}

	if _, err := handlers.DeleteProject(name); err != nil {
		t.Fatalf("DeleteProject failed: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, err := handlers.DescribeProject(name)
		if err != nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("project %s still present after deletion timeout", name)
}

func TestE2E_Unmanaged_OtherResourceMutations_Optional(t *testing.T) {
	requireUnmanagedE2EEnabled(t)
	if !envEnabled("E2E_ALLOW_MUTATION") {
		t.Skip("set E2E_ALLOW_MUTATION=true to run unmanaged mutation e2e tests")
	}

	project := os.Getenv("E2E_PROJECT")

	t.Run("SecurityGroup", func(t *testing.T) {
		name := fmt.Sprintf("csctl-e2e-sg-%d", time.Now().UnixNano())
		sg := &v1.SecurityGroup{
			Metadata: v1.Metadata{
				Name:    name,
				Project: project,
				Annotations: map[string]string{
					"description": "cloudstackctl e2e security group",
				},
			},
		}

		id, err := handlers.ApplySecurityGroup(sg)
		if err != nil {
			t.Fatalf("ApplySecurityGroup failed: %v", err)
		}
		if id == "" {
			t.Fatalf("ApplySecurityGroup returned empty id")
		}

		defer func() { _, _ = handlers.DeleteSecurityGroup(name, project) }()

		if _, err := handlers.DescribeSecurityGroup(name, project, project == ""); err != nil {
			t.Fatalf("DescribeSecurityGroup after create failed: %v", err)
		}

		if _, err := handlers.DeleteSecurityGroup(name, project); err != nil {
			t.Fatalf("DeleteSecurityGroup failed: %v", err)
		}
	})

	t.Run("AffinityGroup", func(t *testing.T) {
		name := fmt.Sprintf("csctl-e2e-ag-%d", time.Now().UnixNano())
		agType := os.Getenv("E2E_AFFINITY_TYPE")
		if agType == "" {
			agType = "host anti-affinity"
		}
		ag := &v1.AffinityGroup{
			Metadata: v1.Metadata{Name: name, Project: project},
			Spec:     v1.AffinitySpec{Type: agType},
		}

		id, err := handlers.ApplyAffinityGroup(ag)
		if err != nil {
			t.Fatalf("ApplyAffinityGroup failed: %v", err)
		}
		if id == "" {
			t.Fatalf("ApplyAffinityGroup returned empty id")
		}

		defer func() { _, _ = handlers.DeleteAffinityGroup(name, project) }()

		if _, err := handlers.DescribeAffinityGroup(name, project, project == ""); err != nil {
			t.Fatalf("DescribeAffinityGroup after create failed: %v", err)
		}

		if _, err := handlers.DeleteAffinityGroup(name, project); err != nil {
			t.Fatalf("DeleteAffinityGroup failed: %v", err)
		}
	})

	t.Run("UserData", func(t *testing.T) {
		name := fmt.Sprintf("csctl-e2e-ud-%d", time.Now().UnixNano())
		ud := &v1.UserData{
			Metadata: v1.Metadata{Name: name, Project: project},
			Spec:     v1.UserDataSpec{Script: "#!/bin/bash\necho e2e\n"},
		}

		id, err := handlers.ApplyUserData(ud)
		if err != nil {
			t.Fatalf("ApplyUserData failed: %v", err)
		}
		if id == "" {
			// Some CloudStack responses may not include id in this flow; describe validates creation.
			debugf("ApplyUserData returned empty id for %s", name)
		}

		defer func() { _, _ = handlers.DeleteUserData(name, project) }()

		if _, err := handlers.DescribeUserData(name, project, project == ""); err != nil {
			t.Fatalf("DescribeUserData after create failed: %v", err)
		}

		if _, err := handlers.DeleteUserData(name, project); err != nil {
			t.Fatalf("DeleteUserData failed: %v", err)
		}
	})
}
