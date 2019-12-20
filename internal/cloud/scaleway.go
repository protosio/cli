package cloud

import (
	"strings"
	"time"

	"github.com/pkg/errors"
	account "github.com/scaleway/scaleway-sdk-go/api/account/v2alpha1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/api/marketplace/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	// "github.com/sirupsen/logrus"
)

const (
	scalewayArch = "x86_64"
	uploadSSHkey = "protos-upload-key"
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
	// create and add ssh key to account
	//

	privKey, pubKey, err := generateSSHkey()
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}
	pubKey = strings.TrimSuffix(pubKey, "\n") + " root@protos.io"

	sshKey, err := sw.accountAPI.CreateSSHKey(&account.CreateSSHKeyRequest{Name: uploadSSHkey, OrganizationID: sw.credentials.organisationID, PublicKey: pubKey})
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway: Failed to add temporary SSH key")
	}
	defer sw.cleanImageSSHkeys(sshKey.ID)

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

	srv, vol, err := sw.createImageUploadVM(imageID)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}
	defer sw.cleanImageUploadVM(srv)

	//
	// connect via SSH, download Protos image and write it to a volume
	//

	signer, err := ssh.ParsePrivateKey(privKey)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway")
	}

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO validate server before?
	}

	log.Info("Trying to connect to Scaleway upload instance over SSH")
	var client *ssh.Client
	tries := 0
	for {
		tries++
		if tries > 20 {
			return errors.New("Failed to add Protos image to Scaleway. Max retries reached while trying to SSH into the upload server")
		}
		client, err = ssh.Dial("tcp", srv.PublicIP.Address.String()+":22", sshConfig) // TODO remove hardocoded port?
		if err != nil {
			log.Infof("Instance not available yet. Waiting 5 seconds... : %s", err.Error())
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
	log.Info("SSH connection initiated")

	//
	// wite Protos image to volume
	//

	log.Info("Downloading Protos image")
	out, err := executeSSHCommand("wget -P /tmp https://releases.protos.io/test/scaleway-efi.iso", client)
	if err != nil {
		log.Errorf("Error downloading Protos VM image: %s", out)
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Error downloading Protos VM image")
	}

	out, err = executeSSHCommand("ls /dev/vdb", client)
	if err != nil {
		log.Errorf("Snapshot volume not found: %s", out)
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Snapshot volume not found")
	}

	log.Info("Writing Protos image to volume")
	out, err = executeSSHCommand("dd if=/tmp/scaleway-efi.iso of=/dev/vdb", client)
	if err != nil {
		log.Errorf("Error while writing image to volume: %s", out)
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Error while writing image to volume")
	}

	//
	// turn off upload VM and dettach volume
	//

	log.Infof("Stopping upload server '%s' (%s)", srv.Name, srv.ID)
	stopReq := &instance.ServerActionAndWaitRequest{
		ServerID: srv.ID,
		Zone:     scw.ZoneNlAms1,
		Action:   instance.ServerActionPoweroff,
	}
	err = sw.instanceAPI.ServerActionAndWait(stopReq)
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Error while stopping upload server")
	}

	_, err = sw.instanceAPI.DetachVolume(&instance.DetachVolumeRequest{Zone: scw.ZoneNlAms1, VolumeID: vol.ID})
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Error while detaching image volume")
	}

	//
	// create snapshot and image
	//

	log.Info("Creating snapshot from volume")
	snapshotResp, err := sw.instanceAPI.CreateSnapshot(&instance.CreateSnapshotRequest{
		VolumeID: vol.ID,
		Name:     "protos-image-snapshot",
		Zone:     scw.ZoneNlAms1,
	})
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Error while creating snapshot from volume")
	}

	log.Info("Creating image from snapshot")
	imageResp, err := sw.instanceAPI.CreateImage(&instance.CreateImageRequest{
		Name:       "protos-image",
		Arch:       instance.ArchX86_64,
		RootVolume: snapshotResp.Snapshot.ID,
		Zone:       scw.ZoneNlAms1,
	})
	if err != nil {
		return errors.Wrap(err, "Failed to add Protos image to Scaleway. Error while creating image from snapshot")
	}
	log.Infof("Protos image '%s' created", imageResp.Image.ID)

	log.Infof("Deleting protos image volume '%s'", vol.ID)
	err = sw.instanceAPI.DeleteVolume(&instance.DeleteVolumeRequest{Zone: scw.ZoneNlAms1, VolumeID: vol.ID})
	if err != nil {
		return errors.Wrap(err, "Error while removing protos image volume. Manual clean might be needed")
	}

	return nil
}

func executeSSHCommand(cmd string, client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", errors.Wrap(err, "Failed to create new sessions")
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		session.Close()
		return "", errors.Wrap(err, "Request for pseudo terminal failed")
	}

	log.Infof("Executing (SSH) command '%s'", cmd)
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), errors.Wrapf(err, "Failed to execute command '%s'", cmd)
	}

	session.Close()

	return string(output), nil

}

func (sw *scaleway) cleanImageSSHkeys(keyID string) {
	err := sw.accountAPI.DeleteSSHKey(&account.DeleteSSHKeyRequest{SSHKeyID: keyID})
	if err != nil {
		log.Error(errors.Wrapf(err, "Failed to clean up Scaleway image upload key with id '%s'", keyID))
	}
	log.Infof("Deleted SSH key '%s'", keyID)
}

func (sw *scaleway) createImageUploadVM(imageID string) (*instance.Server, *instance.Volume, error) {

	//
	// create volume
	//

	size := scw.Size(uint64(10000000000))
	createVolumeReq := &instance.CreateVolumeRequest{
		Name:       "protos-image-uploader",
		VolumeType: "l_ssd",
		Size:       &size,
		Zone:       scw.ZoneNlAms1,
	}

	log.Info("Creating image volume")
	volumeResp, err := sw.instanceAPI.CreateVolume(createVolumeReq)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to create image volume")
	}

	//
	// create server
	//

	volumeMap := make(map[string]*instance.VolumeTemplate)
	volumeTemplate := &instance.VolumeTemplate{
		Size: size,
	}
	volumeMap["0"] = volumeTemplate

	ipreq := true
	req := &instance.CreateServerRequest{
		Name:              "protos-image-uploader",
		Zone:              scw.ZoneNlAms1,
		CommercialType:    "DEV1-S",
		DynamicIPRequired: &ipreq,
		EnableIPv6:        false,
		BootType:          instance.BootTypeLocal,
		Image:             imageID,
		Volumes:           volumeMap,
	}

	srvResp, err := sw.instanceAPI.CreateServer(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to create upload VM")
	}
	log.Infof("Created server '%s' (%s)", srvResp.Server.Name, srvResp.Server.ID)

	//
	// attach volume
	//

	log.Info("Attaching snapshot volume to upload VM")
	attachVolumeReq := &instance.AttachVolumeRequest{
		ServerID: srvResp.Server.ID,
		VolumeID: volumeResp.Volume.ID,
		Zone:     scw.ZoneNlAms1,
	}

	_, err = sw.instanceAPI.AttachVolume(attachVolumeReq)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to attach volume to upload VM")
	}

	//
	// start server
	//

	// default timeout is 5 minutes
	log.Infof("Starting and waiting for server '%s' (%s)", srvResp.Server.Name, srvResp.Server.ID)
	startReq := &instance.ServerActionAndWaitRequest{
		ServerID: srvResp.Server.ID,
		Zone:     scw.ZoneNlAms1,
		Action:   instance.ServerActionPoweron,
	}
	err = sw.instanceAPI.ServerActionAndWait(startReq)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to start upload server")
	}
	log.Infof("Server '%s' (%s) started successfully", srvResp.Server.Name, srvResp.Server.ID)

	//
	// refresh IP info
	//

	srvStatusResp, err := sw.instanceAPI.GetServer(&instance.GetServerRequest{ServerID: srvResp.Server.ID, Zone: scw.ZoneNlAms1})
	if err != nil {
		return nil, nil, errors.Wrap(err, "Failed to retrieve upload VM details")
	}

	return srvStatusResp.Server, volumeResp.Volume, nil
}

func (sw *scaleway) cleanImageUploadVM(srv *instance.Server) {
	srvStatusResp, err := sw.instanceAPI.GetServer(&instance.GetServerRequest{ServerID: srv.ID, Zone: scw.ZoneNlAms1})
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to refresh upload server info"))
		return
	}
	srv = srvStatusResp.Server

	if srv.State == instance.ServerStateRunning {
		// default timeout is 5 minutes
		log.Infof("Stopping and waiting for server '%s' (%s)", srv.Name, srv.ID)
		stopReq := &instance.ServerActionAndWaitRequest{
			ServerID: srv.ID,
			Zone:     scw.ZoneNlAms1,
			Action:   instance.ServerActionPoweroff,
		}
		err = sw.instanceAPI.ServerActionAndWait(stopReq)
		if err != nil {
			log.Error(errors.Wrap(err, "Failed to stop upload server"))
			return
		}
		log.Infof("Server '%s' (%s) stopped successfully", srv.Name, srv.ID)
	}

	for _, vol := range srv.Volumes {
		log.Infof("Deleting volume '%s' for server '%s' (%s)", vol.ID, srv.Name, srv.ID)
		_, err = sw.instanceAPI.DetachVolume(&instance.DetachVolumeRequest{Zone: scw.ZoneNlAms1, VolumeID: vol.ID})
		if err != nil {
			log.Errorf("Failed to dettach volume '%s' for server '%s' (%s): %s", vol.ID, srv.Name, srv.ID, err.Error())
			continue
		}
		err = sw.instanceAPI.DeleteVolume(&instance.DeleteVolumeRequest{Zone: scw.ZoneNlAms1, VolumeID: vol.ID})
		if err != nil {
			log.Errorf("Failed to delete volume '%s' for server '%s' (%s): %s", vol.ID, srv.Name, srv.ID, err.Error())
		}
	}

	log.Infof("Deleting server '%s' (%s)", srv.Name, srv.ID)
	err = sw.instanceAPI.DeleteServer(&instance.DeleteServerRequest{ServerID: srv.ID, Zone: scw.ZoneNlAms1})
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to add Protos image to Scaleway"))
		return
	}
}
