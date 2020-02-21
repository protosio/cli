package cloud

import (
	"log"

	"github.com/pkg/errors"
)

type Type string

func (ct Type) String() string {
	return string(ct)
}

const (
	// DigitalOcean represents the DigitalOcean cloud provider
	DigitalOcean = Type("digitalocean")
	// Scaleway represents the Scaleway cloud provider
	Scaleway = Type("scaleway")
)

// SupportedProviders returns a list of supported cloud providers
func SupportedProviders() []string {
	return []string{Scaleway.String()}
}

// ProviderInfo stores information about a cloud provider
type ProviderInfo struct {
	Name string `storm:"id"`
	Type Type
	Auth map[string]string
}

// Client returns a cloud provider client that can be used to run all the operations exposed by the Provider interface
func (pi ProviderInfo) Client() Provider {
	client, err := NewProvider(pi.Name, pi.Type.String())
	if err != nil {
		log.Fatal(err)
	}
	return client
}

// InstanceInfo holds information about a cloud instance
type InstanceInfo struct {
	VMID      string
	Name      string `storm:"id"`
	KeySeed   []byte
	PublicIP  string
	CloudType Type
	CloudName string
	Location  string
	Volumes   []VolumeInfo
}

// VolumeInfo holds information about a data volume
type VolumeInfo struct {
	VolumeID string
	Name     string
	Size     uint64
}

// MachineSpec holds information about the hardware characteristics of vm or baremetal instance
type MachineSpec struct {
	Cores                uint32  // Nr of cores
	Memory               uint32  // MiB
	DefaultStorage       uint32  // GB
	Bandwidth            uint32  // Mbit
	IncludedDataTransfer uint32  // GB. 0 for unlimited
	Baremetal            bool    // true if machine is bare metal
	PriceMonthly         float32 // no currency conversion at the moment. Each cloud reports this differently
}

// Provider allows interactions with cloud instances and images
type Provider interface {
	// Config methods
	AuthFields() (fields []string)                      // returns the fields that are required to authenticate for a specific cloud provider
	SupportedLocations() (locations []string)           // returns the supported locations for a specific cloud provider
	Init(auth map[string]string, location string) error // a cloud provider always needs to have Init called to configure it
	GetInfo() ProviderInfo                              // returns information that can be stored in the database and allows for re-creation of the provider
	SupportedMachines() (map[string]MachineSpec, error) // returns a map of machine ids and their hardware specifications. A user will choose the machines for their instance

	// Instance methods
	NewInstance(name string, image string, pubKey string, machineType string) (id string, err error)
	DeleteInstance(id string) error
	StartInstance(id string) error
	StopInstance(id string) error
	GetInstanceInfo(id string) (InstanceInfo, error)
	// Image methods
	GetImages() (images map[string]string, err error)
	AddImage(url string, hash string, version string) (id string, err error)
	RemoveImage(name string) error
	// Volume methods
	// - size should by provided in megabytes
	NewVolume(name string, size int) (id string, err error)
	DeleteVolume(id string) error
	AttachVolume(volumeID string, instanceID string) error
	DettachVolume(volumeID string, instanceID string) error
}

// NewProvider creates a new cloud provider client
func NewProvider(cloudName string, cloud string) (Provider, error) {
	var client Provider
	var err error
	cloudType := Type(cloud)
	switch cloudType {
	// case DigitalOcean:
	// 	client, err = newDigitalOceanClient()
	case Scaleway:
		client = newScalewayClient(cloudName)
	default:
		err = errors.Errorf("Cloud '%s' not supported", cloud)
	}
	if err != nil {
		return nil, err
	}
	return client, nil
}

func findInSlice(slice []string, value string) (int, bool) {
	for i, item := range slice {
		if item == value {
			return i, true
		}
	}
	return -1, false
}
