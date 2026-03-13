package v1

import (
	"time"

	"gorm.io/gorm"
)

// APIVersion defines the API version for cloudstackctl resources
const APIVersion = "cloudstackctl/v1"

// Metadata contains resource metadata (name/labels/annotations)
type Metadata struct {
	Name        string            `json:"name" yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty" gorm:"serializer:json"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty" gorm:"serializer:json"`
}

// Status tracks the observed state of a resource in CloudStack
type Status struct {
	ObservedState string    `json:"observedState,omitempty"` // Running/Failed/Pending
	Ready         bool      `json:"ready,omitempty"`         // Health check status
	CloudStackID  string    `json:"cloudStackId,omitempty"`  // External ID in CloudStack
	LastChecked   time.Time `json:"lastChecked,omitempty"`   // Last health check timestamp
	Drift         bool      `json:"drift,omitempty"`         // True if desired != observed state
}

// Application represents a full application/service stack in CloudStack
type Application struct {
	gorm.Model
	APIVersion string          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string          `json:"kind" yaml:"kind"` // "Application"
	Metadata   Metadata        `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       ApplicationSpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status          `json:"status,omitempty" gorm:"embedded"`
}

// ApplicationSpec defines the desired state of an Application
type ApplicationSpec struct {
	ProjectID  string         `yaml:"projectId"`                         // CloudStack project ID
	Components []ComponentRef `yaml:"components" gorm:"serializer:json"` // Dependent components (ordered)
}

// ComponentRef references a Component within an Application
type ComponentRef struct {
	Name               string `yaml:"name"`               // Component name
	VirtualMachineSpec string `yaml:"virtualMachineSpec"` // Reusable VM spec name
	Replicas           int    `yaml:"replicas"`           // Number of VM replicas
}

// Component represents a set of VMs for a specific role (e.g. frontend/backend)
type Component struct {
	gorm.Model
	APIVersion string        `json:"apiVersion" yaml:"apiVersion"`
	Kind       string        `json:"kind" yaml:"kind"` // "Component"
	Metadata   Metadata      `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       ComponentSpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status        `json:"status,omitempty" gorm:"embedded"`
	// EffectiveSpec stores the resolved VM spec after merging overrides (persisted for visibility)
	EffectiveSpec VirtualMachineSpec `json:"effectiveSpec,omitempty" yaml:"effectiveSpec,omitempty" gorm:"serializer:json"`
	// ObservedReplicas tracks how many VMs are currently observed for this component
	ObservedReplicas int `json:"observedReplicas,omitempty" yaml:"observedReplicas,omitempty" gorm:"column:observed_replicas"`
}

// ComponentSpec defines the desired state of a Component
type ComponentSpec struct {
	VirtualMachineSpec string             `yaml:"virtualMachineSpec"` // Reusable VM spec name
	Replicas           int                `yaml:"replicas"`           // Number of VM replicas
	Overrides          ComponentOverrides `yaml:"overrides,omitempty" gorm:"serializer:json"`
}

// ComponentOverrides allows limited, safe overrides to a reused VM spec
type ComponentOverrides struct {
	UserDataRefs   []string `yaml:"userDataRefs,omitempty" gorm:"serializer:json"`
	SSHKeys        []string `yaml:"sshKeys,omitempty" gorm:"serializer:json"`
	SecurityGroups []string `yaml:"securityGroups,omitempty" gorm:"serializer:json"`
	AffinityGroups []string `yaml:"affinityGroups,omitempty" gorm:"serializer:json"`
}

// VirtualMachine represents an individual VM instance in CloudStack
type VirtualMachine struct {
	gorm.Model
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"` // "VirtualMachine"
	Metadata   Metadata           `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       VirtualMachineSpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status             `json:"status,omitempty" gorm:"embedded"`
	// ObservedSpec stores the configuration fetched from CloudStack (observed state)
	ObservedSpec VirtualMachineSpec `json:"observedSpec,omitempty" yaml:"observedSpec,omitempty" gorm:"serializer:json"`
}

// VirtualMachineSpec defines reusable VM configuration (template/offering/network)
type VirtualMachineSpec struct {
	ProjectID       string       `yaml:"projectId"`                                     // CloudStack project ID
	Template        string       `yaml:"template"`                                      // VM template name/ID
	ServiceOffering string       `yaml:"serviceOffering"`                               // VM service offering (size)
	NetworkIDs      []string     `yaml:"networkIds" gorm:"serializer:json"`             // Attached networks
	SSHKeys         []string     `yaml:"sshKeys" gorm:"serializer:json"`                // SSH keys for access
	SecurityGroups  []string     `yaml:"securityGroups" gorm:"serializer:json"`         // Firewall groups
	AffinityGroups  []string     `yaml:"affinityGroups" gorm:"serializer:json"`         // Host/VM affinity rules
	UserDataRefs    []string     `yaml:"userDataRefs,omitempty" gorm:"serializer:json"` // Optional references to UserData resources
	Volumes         []VolumeSpec `yaml:"volumes,omitempty" gorm:"serializer:json"`      // Desired or observed attached volumes
}

// VirtualMachineSpecResource is the persisted wrapper for reusable VM specs
type VirtualMachineSpecResource struct {
	gorm.Model
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"` // "VirtualMachineSpec"
	Metadata   Metadata           `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       VirtualMachineSpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status             `json:"status,omitempty" gorm:"embedded"`
}

// Network represents a CloudStack network resource
type Network struct {
	gorm.Model
	APIVersion string      `json:"apiVersion" yaml:"apiVersion"`
	Kind       string      `json:"kind" yaml:"kind"` // "Network"
	Metadata   Metadata    `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       NetworkSpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status      `json:"status,omitempty" gorm:"embedded"`
}

// NetworkSpec defines the desired state of a Network
type NetworkSpec struct {
	ZoneID            string `yaml:"zoneId"`                      // CloudStack zone ID
	NetworkOfferingID string `yaml:"networkOfferingId,omitempty"` // Network offering ID for creation
	Description       string `yaml:"description,omitempty"`       // Human-friendly description / displayText
	Gateway           string `yaml:"gateway,omitempty"`           // Gateway IP for shared networks
	Netmask           string `yaml:"netmask,omitempty"`           // Netmask for shared networks
	StartIP           string `yaml:"startIp,omitempty"`           // Start IP for static IP range (shared network)
	EndIP             string `yaml:"endIp,omitempty"`             // End IP for static IP range (shared network)
}

// Volume represents a disk attached to a VM in CloudStack
type Volume struct {
	gorm.Model
	APIVersion string     `json:"apiVersion" yaml:"apiVersion"`
	Kind       string     `json:"kind" yaml:"kind"` // "Volume"
	Metadata   Metadata   `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       VolumeSpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status     `json:"status,omitempty" gorm:"embedded"`
}

// VolumeSpec defines the desired state of a Volume
type VolumeSpec struct {
	DiskOffering string `yaml:"diskOffering"`        // Disk offering type (HDD/SSD)
	SizeGB       int    `yaml:"size" gorm:"size_gb"` // Disk size in GB
	ID           string `yaml:"id,omitempty" gorm:"column:volume_id"`
	Name         string `yaml:"name,omitempty"`
}

// SSHKey represents an SSH key pair for VM access in CloudStack
type SSHKey struct {
	gorm.Model
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"` // "SSHKey"
	Metadata   Metadata `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Status     Status   `json:"status,omitempty" gorm:"embedded"`
}

// SecurityGroup represents a firewall rule set for VMs in CloudStack
type SecurityGroup struct {
	gorm.Model
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"` // "SecurityGroup"
	Metadata   Metadata `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Status     Status   `json:"status,omitempty" gorm:"embedded"`
}

// AffinityGroup represents host/VM affinity/anti-affinity rules in CloudStack
type AffinityGroup struct {
	gorm.Model
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"` // "AffinityGroup"
	Metadata   Metadata     `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       AffinitySpec `json:"spec" yaml:"spec" gorm:"embedded"`
	Status     Status       `json:"status,omitempty" gorm:"embedded"`
}

// AffinitySpec defines the type of affinity rule
type AffinitySpec struct {
	Type string `yaml:"type"` // hostAntiAffinity/hostAffinity
}

// UserData represents initialization scripts for VMs in CloudStack
type UserData struct {
	gorm.Model
	APIVersion string       `json:"apiVersion" yaml:"apiVersion"`
	Kind       string       `json:"kind" yaml:"kind"` // "UserData"
	Metadata   Metadata     `json:"metadata" yaml:"metadata" gorm:"embedded"`
	Spec       UserDataSpec `json:"spec" yaml:"spec" gorm:"embedded"`
}

// UserDataSpec defines the initialization script content
type UserDataSpec struct {
	Script string `yaml:"script"` // Base64-encoded user data script
}

// TableName overrides to ensure consistent table names across DB operations
func (Application) TableName() string {
	return "applications"
}

func (Component) TableName() string {
	return "components"
}

func (VirtualMachine) TableName() string {
	return "virtual_machines"
}

func (VirtualMachineSpecResource) TableName() string {
	return "vm_specs"
}
