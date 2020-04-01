package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	"github.com/protosio/cli/internal/release"
	ssh "github.com/protosio/cli/internal/ssh"
	"github.com/protosio/cli/internal/user"
	pclient "github.com/protosio/protos/pkg/client"
	"github.com/urfave/cli/v2"
)

var machineType string

var cmdInstance *cli.Command = &cli.Command{
	Name:  "instance",
	Usage: "Manage Protos instances",
	Subcommands: []*cli.Command{
		{
			Name:  "ls",
			Usage: "List instances",
			Action: func(c *cli.Context) error {
				return listInstances()
			},
		},
		{
			Name:      "deploy",
			ArgsUsage: "<name>",
			Usage:     "Deploy a new Protos instance",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "cloud",
					Usage:       "Specify which `CLOUD` to deploy the instance on",
					Required:    true,
					Destination: &cloudName,
				},
				&cli.StringFlag{
					Name:        "location",
					Usage:       "Specify one of the supported `LOCATION`s to deploy the instance in (cloud specific)",
					Required:    true,
					Destination: &cloudLocation,
				},
				&cli.StringFlag{
					Name:        "version",
					Usage:       "Specify Protos `VERSION` to deploy",
					Required:    false,
					Destination: &protosVersion,
				},
				&cli.StringFlag{
					Name:        "type",
					Usage:       "Specify cloud machine type `TYPE` to deploy. Get it from 'cloud info' subcommand",
					Required:    true,
					Destination: &machineType,
				},
			},
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				releases, err := getProtosAvailableReleases()
				if err != nil {
					return err
				}
				var release release.Release
				if protosVersion == "" {
					release, err = releases.GetLatest()
					if err != nil {
						return err
					}
				} else {
					release, err = releases.GetVersion(protosVersion)
					if err != nil {
						return err
					}
				}

				_, err = deployInstance(name, cloudName, cloudLocation, release, machineType)
				return err
			},
		},
		{
			Name:      "delete",
			ArgsUsage: "<name>",
			Usage:     "Delete instance",
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				return deleteInstance(name)
			},
		},
		{
			Name:      "start",
			ArgsUsage: "<name>",
			Usage:     "Power on instance",
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				return startInstance(name)
			},
		},
		{
			Name:      "stop",
			ArgsUsage: "<name>",
			Usage:     "Power off instance",
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				return stopInstance(name)
			},
		},
		{
			Name:      "tunnel",
			ArgsUsage: "<name>",
			Usage:     "Creates SSH encrypted tunnel to instance dashboard",
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				return tunnelInstance(name)
			},
		},
		{
			Name:      "key",
			ArgsUsage: "<name>",
			Usage:     "Prints to stdout the SSH key associated with the instance",
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				return keyInstance(name)
			},
		},
	},
}

//
// Instance methods
//

func listInstances() error {
	instances, err := envi.DB.GetAllInstances()
	if err != nil {
		return err
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 2, ' ', 0)

	defer w.Flush()

	fmt.Fprintf(w, " %s\t%s\t%s\t%s\t%s\t%s\t", "Name", "IP", "Cloud", "VM ID", "Location", "Status")
	fmt.Fprintf(w, "\n %s\t%s\t%s\t%s\t%s\t%s\t", "----", "--", "-----", "-----", "--------", "------")
	for _, instance := range instances {
		fmt.Fprintf(w, "\n %s\t%s\t%s\t%s\t%s\t%s\t", instance.Name, instance.PublicIP, instance.CloudName, instance.VMID, instance.Location, "n/a")
	}
	fmt.Fprint(w, "\n")
	return nil
}

func deployInstance(instanceName string, cloudName string, cloudLocation string, release release.Release, machineType string) (cloud.InstanceInfo, error) {

	// init cloud
	provider, err := envi.DB.GetCloud(cloudName)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Could not retrieve cloud '%s'", cloudName)
	}
	client := provider.Client()
	err = client.Init(provider.Auth)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to connect to cloud provider '%s'(%s) API", cloudName, provider.Type.String())
	}

	// validate machine type
	supportedMachineTypes, err := client.SupportedMachines(cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, err
	}
	if _, found := supportedMachineTypes[machineType]; !found {
		return cloud.InstanceInfo{}, errors.Errorf("Machine type '%s' is not valid for cloud provider '%s'. The following types are supported: \n%s", machineType, string(provider.Type), createMachineTypesString(supportedMachineTypes))
	}

	// add image
	imageID := ""
	images, err := client.GetImages()
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to deploy Protos instance")
	}
	for id, img := range images {
		if img.Location == cloudLocation && img.Name == release.Version {
			imageID = id
			break
		}
	}
	if imageID != "" {
		log.Infof("Found Protos image version '%s' in your cloud account", release.Version)
	} else {
		// upload protos image
		if image, found := release.CloudImages[string(provider.Type)]; found {
			log.Infof("Protos image version '%s' not in your infra cloud account. Adding it.", release.Version)
			imageID, err = client.AddImage(image.URL, image.Digest, release.Version, cloudLocation)
			if err != nil {
				return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to deploy Protos instance")
			}
		} else {
			return cloud.InstanceInfo{}, errors.Errorf("Could not find a Protos version '%s' release for cloud '%s'", release.Version, string(provider.Type))
		}
	}

	// create SSH key used for instance
	log.Info("Generating SSH key for the new VM instance")
	key, err := ssh.GenerateKey()
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to deploy Protos instance")
	}

	// deploy a protos instance
	log.Infof("Deploying instance '%s' of type '%s', using Protos version '%s' (image id '%s')", instanceName, machineType, release.Version, imageID)
	vmID, err := client.NewInstance(instanceName, imageID, key.Public(), machineType, cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to deploy Protos instance")
	}
	log.Infof("Instance with ID '%s' deployed", vmID)

	// get instance info
	instanceInfo, err := client.GetInstanceInfo(vmID, cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to get Protos instance info")
	}
	// save of the instance information
	err = envi.DB.SaveInstance(instanceInfo)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to save instance '%s'", instanceName)
	}

	// create protos data volume
	log.Infof("Creating data volume for Protos instance '%s'", instanceName)
	volumeID, err := client.NewVolume(instanceName, 30000, cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to create data volume")
	}

	// attach volume to instance
	err = client.AttachVolume(volumeID, vmID, cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to attach volume to instance '%s'", instanceName)
	}

	// start protos instance
	log.Infof("Starting Protos instance '%s'", instanceName)
	err = client.StartInstance(vmID, cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to start Protos instance")
	}

	// get instance info again
	instanceInfo, err = client.GetInstanceInfo(vmID, cloudLocation)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to get Protos instance info")
	}
	// final save of the instance information
	instanceInfo.KeySeed = key.Seed()
	err = envi.DB.SaveInstance(instanceInfo)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to save instance '%s'", instanceName)
	}

	// wait for the instance to be up
	log.Infof("Creating SSH tunnel to instance '%s'", instanceName)
	tunnel := ssh.NewTunnel(instanceInfo.PublicIP+":22", "root", key.SSHAuth(), "localhost:8080", log)
	tries := 0
	var localPort int
	for {
		localPort, err = tunnel.Start()
		if err != nil {
			lerr := errors.Wrap(err, "Error while creating the SSH tunnel")
			if tries == 20 {
				return cloud.InstanceInfo{}, lerr
			}
			log.Debugf("Waiting for instance to be reachable: %v", lerr)
			tries++
			time.Sleep(3 * time.Second)
			continue
		} else {
			break
		}
	}

	user, err := user.Get(envi)
	if err != nil {
		return cloud.InstanceInfo{}, err
	}

	// do the initialization
	protos := pclient.NewInitClient(fmt.Sprintf("locahost:%d", localPort), user.Username, user.Password, user.Domain)
	err = protos.InitInstance()
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Error while doing the instance initialization")
	}

	// close the SSH tunnel
	err = tunnel.Close()
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Error while terminating the SSH tunnel")
	}

	return instanceInfo, nil
}

func deleteInstance(name string) error {
	instance, err := envi.DB.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	cloudInfo, err := envi.DB.GetCloud(instance.CloudName)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloudInfo.Client()
	err = client.Init(cloudInfo.Auth)
	if err != nil {
		return errors.Wrapf(err, "Could not init cloud '%s'", name)
	}

	log.Infof("Stopping instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.StopInstance(instance.VMID, cloudLocation)
	if err != nil {
		return errors.Wrapf(err, "Could not stop instance '%s'", name)
	}
	vmInfo, err := client.GetInstanceInfo(instance.VMID, cloudLocation)
	if err != nil {
		return errors.Wrapf(err, "Failed to get details for instance '%s'", name)
	}
	log.Infof("Deleting instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.DeleteInstance(instance.VMID, cloudLocation)
	if err != nil {
		return errors.Wrapf(err, "Could not delete instance '%s'", name)
	}
	for _, vol := range vmInfo.Volumes {
		log.Infof("Deleting volume '%s' (%s) for instance '%s'", vol.Name, vol.VolumeID, name)
		err = client.DeleteVolume(vol.VolumeID, cloudLocation)
		if err != nil {
			log.Errorf("Failed to delete volume '%s': %s", vol.Name, err.Error())
		}
	}
	return envi.DB.DeleteInstance(name)
}

func startInstance(name string) error {
	instance, err := envi.DB.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	cloudInfo, err := envi.DB.GetCloud(instance.CloudName)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloudInfo.Client()
	err = client.Init(cloudInfo.Auth)
	if err != nil {
		return errors.Wrapf(err, "Could not init cloud '%s'", name)
	}

	log.Infof("Starting instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.StartInstance(instance.VMID, cloudLocation)
	if err != nil {
		return errors.Wrapf(err, "Could not start instance '%s'", name)
	}
	return nil
}

func stopInstance(name string) error {
	instance, err := envi.DB.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	cloudInfo, err := envi.DB.GetCloud(instance.CloudName)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloudInfo.Client()
	err = client.Init(cloudInfo.Auth)
	if err != nil {
		return errors.Wrapf(err, "Could not init cloud '%s'", name)
	}

	log.Infof("Stopping instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.StopInstance(instance.VMID, cloudLocation)
	if err != nil {
		return errors.Wrapf(err, "Could not stop instance '%s'", name)
	}
	return nil
}

func tunnelInstance(name string) error {
	instanceInfo, err := envi.DB.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	if len(instanceInfo.KeySeed) == 0 {
		return errors.Errorf("Instance '%s' is missing its SSH key", name)
	}
	key, err := ssh.NewKeyFromSeed(instanceInfo.KeySeed)
	if err != nil {
		return errors.Wrapf(err, "Instance '%s' has an invalid SSH key", name)
	}

	log.Infof("Creating SSH tunnel to instance '%s', using ip '%s'", instanceInfo.Name, instanceInfo.PublicIP)
	tunnel := ssh.NewTunnel(instanceInfo.PublicIP+":22", "root", key.SSHAuth(), "localhost:8080", log)
	localPort, err := tunnel.Start()
	if err != nil {
		return errors.Wrap(err, "Error while creating the SSH tunnel")
	}

	quit := make(chan interface{}, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go catchSignals(sigs, quit)

	log.Infof("SSH tunnel ready. Use 'http://localhost:%d/' to access the instance dashboard. Once finished, press CTRL+C to terminate the SSH tunnel", localPort)

	// waiting for a SIGTERM or SIGINT
	<-quit

	log.Info("CTRL+C received. Terminating the SSH tunnel")
	err = tunnel.Close()
	if err != nil {
		return errors.Wrap(err, "Error while terminating the SSH tunnel")
	}
	log.Info("SSH tunnel terminated successfully")
	return nil
}

func keyInstance(name string) error {
	instanceInfo, err := envi.DB.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	if len(instanceInfo.KeySeed) == 0 {
		return errors.Errorf("Instance '%s' is missing its SSH key", name)
	}
	key, err := ssh.NewKeyFromSeed(instanceInfo.KeySeed)
	if err != nil {
		return errors.Wrapf(err, "Instance '%s' has an invalid SSH key", name)
	}
	fmt.Print(key.EncodePrivateKeytoPEM())
	return nil
}
