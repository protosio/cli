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

// Client allows interactions with cloud instances and images
type Client interface {
	NewInstance(name string, image string, pubKey string) (string, error)
	DeleteInstance(id string) error
	StartInstance(id string) error
	StopInstance(id string) error
	GetImages() (map[string]string, error)
	AddImage(url string, hash string) (string, error)
	RemoveImage(name string) error
	NewVolume() (string, error)
	DeleteVolume(id string) error
	AuthFields() []string
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
