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
	NewInstance(name string, image string, pubKey string) (id string, err error)
	DeleteInstance(id string) error
	StartInstance(id string) error
	StopInstance(id string) error
	GetInstanceInfo(id string) (InstanceInfo, error)
	GetImages() (images map[string]string, err error)
	AddImage(url string, hash string) (id string, err error)
	RemoveImage(name string) error
	NewVolume() (id string, err error)
	DeleteVolume(id string) error
	AttachVolume(volumeID string, instanceID string) error
	DettachVolume(volumeID string, instanceID string) error
	AuthFields() (fields []string)
	Init(auth map[string]string) error
}

// NewClient creates a new cloud provider client
func NewClient(cloud string) (Client, error) {
	var client Client
	var err error
	switch cloud {
	// case DigitalOcean:
	// 	client, err = newDigitalOceanClient()
	case Scaleway:
		client, err = newScalewayClient()
	default:
		err = errors.Errorf("Cloud '%s' not supported", cloud)
	}
	if err != nil {
		return nil, err
	}
	return client, nil
}
