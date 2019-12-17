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
	NewInstance()
	DeleteInstance()
	StartInstance()
	StopInstance()
	AddImage(url string, hash string) error
	RemoveImage()
	AuthFields() []string
	Init(auth map[string]string) error
}

// NewClient creates a new cloud provider client
func NewClient(cloud string) (Client, error) {
	var client Client
	var err error
	switch cloud {
	case DigitalOcean:
		client, err = newDigitalOceanClient()
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
