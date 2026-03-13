package yamlutil

import (
	v1 "cloudstackctl/apis/v1"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// ParseYAML parses a YAML file into the correct resource type based on Kind
func ParseYAML(filePath string) (interface{}, error) {
	// Read YAML file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// First pass: extract only API version and Kind
	var metadata struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	// Second pass: parse into full resource type based on Kind
	var resource interface{}
	switch metadata.Kind {
	case "Application":
		resource = &v1.Application{}
	case "Component":
		resource = &v1.Component{}
	case "VirtualMachine":
		resource = &v1.VirtualMachine{}
	case "Network":
		resource = &v1.Network{}
	case "Volume":
		resource = &v1.Volume{}
	case "SSHKey":
		resource = &v1.SSHKey{}
	case "SecurityGroup":
		resource = &v1.SecurityGroup{}
	case "AffinityGroup":
		resource = &v1.AffinityGroup{}
	case "UserData":
		resource = &v1.UserData{}
	default:
		return nil, fmt.Errorf("unknown Kind: %s", metadata.Kind)
	}

	// Unmarshal full YAML into resource
	if err := yaml.Unmarshal(data, resource); err != nil {
		return nil, err
	}

	return resource, nil
}
