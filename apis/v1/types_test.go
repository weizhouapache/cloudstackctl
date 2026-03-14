package v1

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestNetworkSpecYAMLUnmarshal(t *testing.T) {
	data := []byte(`apiVersion: cloudstackctl/v1
kind: Network
metadata:
  name: test-net
spec:
	zone: zone-1
	networkOffering: offer-123
  gateway: 10.0.0.1
  netmask: 255.255.255.0
  startIp: 10.0.0.10
  endIp: 10.0.0.20
`)

	var n Network
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
