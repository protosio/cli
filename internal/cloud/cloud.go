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

// Client allows interactions with cloud instances and images
type Client interface {
	NewInstance()
	DeleteInstance()
	StartInstance()
	StopInstance()
	AddImage()
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
		return nil, errors.Errorf("Cloud '%s' not supported", cloud)
	}
	if err != nil {
		return nil, err
	}
	return client, nil
}
