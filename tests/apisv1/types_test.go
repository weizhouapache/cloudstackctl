package v1_test

import (
	"testing"

	v1 "cloudstackctl/apis/v1"

	"sigs.k8s.io/yaml"
)

func TestNetworkSpecYAMLUnmarshal(t *testing.T) {
	data := []byte("apiVersion: cloudstackctl/v1\n" +
		"kind: Network\n" +
		"metadata:\n" +
		"  name: test-net\n" +
		"spec:\n" +
		"  zone: zone-1\n" +
		"  networkOffering: offer-123\n" +
		"  gateway: 10.0.0.1\n" +
		"  netmask: 255.255.255.0\n" +
		"  startIp: 10.0.0.10\n" +
		"  endIp: 10.0.0.20\n")

	var n v1.Network
	if err := yaml.Unmarshal(data, &n); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if n.Metadata.Name != "test-net" {
		t.Fatalf("unexpected metadata.name: %q", n.Metadata.Name)
	}
	if n.Spec.Zone != "zone-1" {
		t.Fatalf("unexpected zone: %q", n.Spec.Zone)
	}
	if n.Spec.NetworkOffering != "offer-123" {
		t.Fatalf("unexpected networkOffering: %q", n.Spec.NetworkOffering)
	}
	if n.Spec.Gateway != "10.0.0.1" {
		t.Fatalf("unexpected gateway: %q", n.Spec.Gateway)
	}
	if n.Spec.Netmask != "255.255.255.0" {
		t.Fatalf("unexpected netmask: %q", n.Spec.Netmask)
	}
	if n.Spec.StartIP != "10.0.0.10" {
		t.Fatalf("unexpected startIp: %q", n.Spec.StartIP)
	}
	if n.Spec.EndIP != "10.0.0.20" {
		t.Fatalf("unexpected endIp: %q", n.Spec.EndIP)
	}
}
