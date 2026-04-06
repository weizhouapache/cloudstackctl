package cli_test

import (
	"testing"

	"cloudstackctl/cmd/cli"
)

func TestNormalizeResourceType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "app", want: "Application"},
		{in: " apps ", want: "Application"},
		{in: "comp", want: "Component"},
		{in: "vms", want: "VirtualMachine"},
		{in: "vmspec", want: "VirtualMachineSpec"},
		{in: "nets", want: "Network"},
		{in: "vol", want: "Volume"},
		{in: "keys", want: "SSHKey"},
		{in: "userdata", want: "UserData"},
		{in: "ag", want: "AffinityGroup"},
		{in: "sg", want: "SecurityGroup"},
		{in: "proj", want: "Project"},
		{in: "unknown-kind", want: "unknown-kind"},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := cli.NormalizeResourceType(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeResourceType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
