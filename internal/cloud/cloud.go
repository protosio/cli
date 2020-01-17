package cloud

import (
	"github.com/pkg/errors"
)

const (
	// DigitalOcean represents the DigitalOcean cloud provider
	DigitalOcean = "digitalocean"
	// Scaleway represents the Scaleway cloud provider
	Scaleway = "scaleway"
)

// SupportedProviders returns a list of supported cloud providers
func SupportedProviders() []string {
	return []string{Scaleway}
}

// InstanceInfo holds information about a cloud instance
type InstanceInfo struct {
	ID       string
	Name     string
	PublicIP string
	Location string
}

// Client allows interactions with cloud instances and images
type Client interface {
	// Config methods
	AuthFields() (fields []string)                      // returns the fields that are required to authenticate for a specific cloud provider
	SupportedLocations() (locations []string)           // returns the supported locations for a specific cloud provider
	Init(auth map[string]string, location string) error // a cloud provider always needs to have Init called to configure it
	// Instance methods
	NewInstance(name string, image string, pubKey string) (id string, err error)
	DeleteInstance(id string) error
	StartInstance(id string) error
	StopInstance(id string) error
	GetInstanceInfo(id string) (InstanceInfo, error)
	// Image methods
	GetImages() (images map[string]string, err error)
	AddImage(url string, hash string) (id string, err error)
	RemoveImage(name string) error
	// Volume methods
	// - size should by provided in megabytes
	NewVolume(name string, size int) (id string, err error)
	DeleteVolume(id string) error
	AttachVolume(volumeID string, instanceID string) error
	DettachVolume(volumeID string, instanceID string) error
}

// NewClient creates a new cloud provider client
func NewClient(cloud string) (Client, error) {
	var client Client
	var err error
	switch cloud {
	// case DigitalOcean:
	// 	client, err = newDigitalOceanClient()
	case Scaleway:
		client = newScalewayClient()
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
