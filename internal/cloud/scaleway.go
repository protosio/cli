package cloud

import (
	"github.com/pkg/errors"
	account "github.com/scaleway/scaleway-sdk-go/api/account/v2alpha1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/api/marketplace/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	log "github.com/sirupsen/logrus"
	// "github.com/sirupsen/logrus"
)

const (
	scalewayArch = "x86_64"
)

type scalewayCredentials struct {
	organisationID string
	accessKey      string
	secretKey      string
}

type scaleway struct {
	credentials    *scalewayCredentials
	client         *scw.Client
	instanceAPI    *instance.API
	accountAPI     *account.API
	marketplaceAPI *marketplace.API
}

func newScalewayClient() (*scaleway, error) {
	scaleway := &scaleway{}

	return scaleway, nil

}

func (sw *scaleway) NewInstance()    {}
func (sw *scaleway) DeleteInstance() {}
func (sw *scaleway) StartInstance()  {}
func (sw *scaleway) StopInstance()   {}
func (sw *scaleway) RemoveImage()    {}

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
	sw.marketplaceAPI = marketplace.NewAPI(sw.client)
	_, err = sw.accountAPI.ListSSHKeys(&account.ListSSHKeysRequest{})
	if err != nil {
		return errors.Wrap(err, "Failed to init Scaleway client")
	}
	return nil
}

func (sw *scaleway) getUploadImageID(zone scw.Zone) (string, error) {
	resp, err := sw.marketplaceAPI.ListImages(&marketplace.ListImagesRequest{})
	if err != nil {
		return "", errors.Wrap(err, "Failed to retrieve available images from Scaleway")
	}
	for _, img := range resp.Images {
		if img.Name == "Ubuntu Bionic" {
			for _, ver := range img.Versions {
				for _, li := range ver.LocalImages {
					if li.Arch == scalewayArch && li.Zone == zone {
						return li.ID, nil
					}
				}
			}
		}
	}
	return "", errors.Errorf("Ubuntu Bionic image in zone '%s' not found", scw.ZoneFrPar1)
}

func (sw *scaleway) AddImage(url string, hash string) error {

	//
	// find correct image
	//

	imageID, err := sw.getUploadImageID(scw.ZoneNlAms1)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}

	log.Infof("Using image '%s' for adding Protos image to Scaleway", imageID)

	//
	// create upload server
	//

	ipreq := true
	req := &instance.CreateServerRequest{
		Name:              "protos-image-uploader",
		Zone:              scw.ZoneNlAms1,
		CommercialType:    "DEV1-S",
		DynamicIPRequired: &ipreq,
		EnableIPv6:        false,
		BootType:          instance.BootTypeLocal,
		Image:             imageID,
	}

	//
	// create and start server
	//

	srvResp, err := sw.instanceAPI.CreateServer(req)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}
	log.Infof("Created server '%s' (%s)", srvResp.Server.Name, srvResp.Server.ID)

	// default timeout is 5 minutes
	log.Infof("Starting and waiting for server '%s' (%s)", srvResp.Server.Name, srvResp.Server.ID)
	startReq := &instance.ServerActionAndWaitRequest{
		ServerID: srvResp.Server.ID,
		Zone:     scw.ZoneNlAms1,
		Action:   instance.ServerActionPoweron,
	}
	err = sw.instanceAPI.ServerActionAndWait(startReq)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}
	log.Infof("Server '%s' (%s) started successfully", srvResp.Server.Name, srvResp.Server.ID)

	//
	// connect via SSH, download Protos image and write it to a volume
	//

	srvStatusResp, err := sw.instanceAPI.GetServer(&instance.GetServerRequest{ServerID: srvResp.Server.ID, Zone: scw.ZoneNlAms1})
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}
	log.Info(srvStatusResp.Server.PublicIP)
	log.Info(srvStatusResp.Server.PrivateIP)

	//
	// create Protos image from volume snapshot
	//

	//
	// power off and delete the server
	//

	// default timeout is 5 minutes
	log.Infof("Stopping and waiting for server '%s' (%s)", srvResp.Server.Name, srvResp.Server.ID)
	stopReq := &instance.ServerActionAndWaitRequest{
		ServerID: srvResp.Server.ID,
		Zone:     scw.ZoneNlAms1,
		Action:   instance.ServerActionPoweroff,
	}
	err = sw.instanceAPI.ServerActionAndWait(stopReq)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}
	log.Infof("Server '%s' (%s) stopped successfully", srvResp.Server.Name, srvResp.Server.ID)

	log.Infof("Deleting server '%s' (%s)", srvResp.Server.Name, srvResp.Server.ID)
	err = sw.instanceAPI.DeleteServer(&instance.DeleteServerRequest{ServerID: srvResp.Server.ID, Zone: scw.ZoneNlAms1})
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}

	return nil
}
