package cloud

import (
	"github.com/pkg/errors"
	account "github.com/scaleway/scaleway-sdk-go/api/account/v2alpha1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

type scalewayCredentials struct {
	organisationID string
	accessKey      string
	secretKey      string
}

type scaleway struct {
	credentials *scalewayCredentials
	client      *scw.Client
	instanceAPI *instance.API
	accountAPI  *account.API
}

func newScalewayClient() (*scaleway, error) {
	scaleway := &scaleway{}

	return scaleway, nil

}

func (sw *scaleway) NewInstance()    {}
func (sw *scaleway) DeleteInstance() {}
func (sw *scaleway) StartInstance()  {}
func (sw *scaleway) StopInstance()   {}
func (sw *scaleway) AddImage()       {}

func (sw *scaleway) AuthFields() []string {
	return []string{"ORGANISATION_ID", "ACCESS_KEY", "SECRET_KEY"}
}

func (sw *scaleway) Init(auth map[string]string) error {
	var err error

	scwCredentials := &scalewayCredentials{}
	for k, v := range auth {
		switch k {
		case "ORGANISATION_ID":
			scwCredentials.organisationID = v
		case "ACCESS_KEY":
			scwCredentials.accessKey = v
		case "SECRET_KEY":
			scwCredentials.secretKey = v
		default:
			return errors.Errorf("Credentials field '%s' not supported for Scaleway cloud provider", k)
		}
		if v == "" {
			return errors.Errorf("Credentials field '%s' is empty", k)
		}
	}

	sw.credentials = scwCredentials
	sw.client, err = scw.NewClient(
		scw.WithDefaultOrganizationID(scwCredentials.organisationID),
		scw.WithAuth(scwCredentials.accessKey, scwCredentials.secretKey),
	)
	if err != nil {
		return errors.Wrap(err, "Failed to init Scaleway client")
	}

	sw.instanceAPI = instance.NewAPI(sw.client)
	sw.accountAPI = account.NewAPI(sw.client)
	_, err = sw.accountAPI.ListSSHKeys(&account.ListSSHKeysRequest{})
	if err != nil {
		return errors.Wrap(err, "Failed to init Scaleway client")
	}
	return nil
}
