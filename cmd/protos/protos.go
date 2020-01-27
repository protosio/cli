package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	"github.com/protosio/cli/internal/db"
	ssh "github.com/protosio/cli/internal/ssh"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log *logrus.Logger
var dbp db.DB

func main() {
	log = logrus.New()
	var loglevel string
	var cloudName string
	app := &cli.App{
		Name:    "protos",
		Usage:   "Command-line client for Protos",
		Version: "0.0.0-dev",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "log, l",
				Value:       "info",
				Usage:       "Log level: warn, info, debug",
				Destination: &loglevel,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initializes Protos locally and deploys an instance in one of the supported clouds",
				Subcommands: []*cli.Command{
					{
						Name:  "db",
						Usage: "Initialize local database",
						Action: func(c *cli.Context) error {
							return protosDBInit()
						},
					},
					{
						Name:  "full",
						Usage: "Initialize a protos instance. Created local db, adds a cloud provider and a Protos instance.",
						Action: func(c *cli.Context) error {
							return protosFullInit()
						},
					},
				},
			},
			{
				Name:  "cloud",
				Usage: "Manage clouds",
				Subcommands: []*cli.Command{
					{
						Name:  "ls",
						Usage: "List existing cloud provider accounts",
						Action: func(c *cli.Context) error {
							return listCloudProviders()
						},
					},
					{
						Name:      "add",
						ArgsUsage: "<name>",
						Usage:     "Add a new cloud provider account",
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							_, err := addCloudProvider(name)
							return err
						},
					},
					{
						Name:      "delete",
						ArgsUsage: "<name>",
						Usage:     "Delete an existing cloud provider account",
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							return deleteCloudProvider(name)
						},
					},
					{
						Name:      "info",
						ArgsUsage: "<name>",
						Usage:     "Prints info about cloud provider account and checks if the API is reachable",
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							return infoCloudProvider(name)
						},
					},
				},
			},
			{
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
						},
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							_, err := addInstance(name, cloudName)
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
			},
		},
	}

	app.Before = func(c *cli.Context) error {
		level, err := logrus.ParseLevel(loglevel)
		if err != nil {
			return err
		}
		log.SetLevel(level)
		config(c.Args().First())
		return nil
	}

	app.After = func(c *cli.Context) error {
		if dbp != nil {
			return dbp.Close()
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

type userDetails struct {
	Username        string
	Name            string
	Password        string
	PasswordConfirm string
	Domain          string
}

func transformCredentials(creds map[string]interface{}) map[string]string {
	transformed := map[string]string{}
	for name, val := range creds {
		transformed[name] = val.(string)
	}
	return transformed
}

func catchSignals(sigs chan os.Signal, quit chan interface{}) {
	<-sigs
	quit <- true
}

func config(currentCmd string) {
	var err error
	if currentCmd != "init" {
		dbp, err = db.Open("")
		if err != nil {
			log.Fatal(err)
		}
	}
}

//
//  Cloud provider methods
//

func listCloudProviders() error {
	clouds, err := dbp.GetAllClouds()
	if err != nil {
		return err
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 16, 16, 0, '\t', 0)

	defer w.Flush()

	fmt.Fprintf(w, " %s\t%s\t", "Name", "Type")
	fmt.Fprintf(w, "\n %s\t%s\t", "----", "----")
	for _, cl := range clouds {
		fmt.Fprintf(w, "\n %s\t%s\t", cl.Name, cl.Type)
	}
	fmt.Fprint(w, "\n")
	return nil
}

func addCloudProvider(cloudName string) (cloud.Provider, error) {
	// select cloud provider
	var cloudType string
	cloudProviderSelect := surveySelect(cloud.SupportedProviders(), "Choose one of the following supported cloud providers:")
	err := survey.AskOne(cloudProviderSelect, &cloudType)
	if err != nil {
		return nil, err
	}

	// create new cloud provider
	client, err := cloud.NewProvider(cloudName, cloudType)
	if err != nil {
		return nil, err
	}

	// get cloud provider credentials
	cloudCredentials := map[string]interface{}{}
	credFields := client.AuthFields()
	credentialsQuestions := getCloudCredentialsQuestions(cloudType, credFields)

	err = survey.Ask(credentialsQuestions, &cloudCredentials)
	if err != nil {
		return nil, err
	}

	// get cloud provider location
	var cloudLocation string
	supportedLocations := client.SupportedLocations()
	cloudLocationQuestions := surveySelect(supportedLocations, fmt.Sprintf("Choose one of the following supported locations for '%s':", cloudType))
	err = survey.AskOne(cloudLocationQuestions, &cloudLocation)
	if err != nil {
		return nil, err
	}

	// init cloud client
	err = client.Init(transformCredentials(cloudCredentials), cloudLocation)
	if err != nil {
		return nil, err
	}

	// save the cloud provider in the db
	cloudProviderInfo := client.GetInfo()
	err = dbp.SaveCloud(cloudProviderInfo)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to save cloud provider info")
	}

	return client, nil
}

func deleteCloudProvider(name string) error {
	return dbp.DeleteCloud(name)
}

func infoCloudProvider(name string) error {
	cloud, err := dbp.GetCloud(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloud.Client()
	locations := client.SupportedLocations()
	err = client.Init(cloud.Auth, locations[0])
	if err != nil {
		return errors.Wrapf(err, "Failed to connect to cloud provider '%s'(%s) API", name, cloud.Type.String())
	}
	fmt.Printf("Name: %s\n", cloud.Name)
	fmt.Printf("Type: %s\n", cloud.Type.String())
	fmt.Printf("Supported locations: %s\n", strings.Join(locations, " | "))
	if err != nil {
		fmt.Printf("Status: NOT OK (%s)\n", err.Error())
	} else {
		fmt.Printf("Status: OK - API reachable\n")
	}
	return nil
}

//
// Instance methods
//

func listInstances() error {
	instances, err := dbp.GetAllInstances()
	if err != nil {
		return err
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 8, 8, 0, '\t', 0)

	defer w.Flush()

	fmt.Fprintf(w, " %s\t%s\t%s\t%s\t%s\t", "Name", "IP", "Cloud", "VM ID", "Status")
	fmt.Fprintf(w, "\n %s\t%s\t%s\t%s\t%s\t", "----", "--", "-----", "-----", "------")
	for _, instance := range instances {
		fmt.Fprintf(w, "\n %s\t%s\t%s\t%s\t%s\t", instance.Name, instance.PublicIP, instance.CloudName, instance.VMID, "n/a")
	}
	fmt.Fprint(w, "\n")
	return nil
}

func addInstance(instanceName string, cloudName string) (cloud.InstanceInfo, error) {

	// init cloud
	provider, err := dbp.GetCloud(cloudName)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Could not retrieve cloud '%s'", cloudName)
	}
	client := provider.Client()
	locations := client.SupportedLocations()
	err = client.Init(provider.Auth, locations[0])
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to connect to cloud provider '%s'(%s) API", cloudName, provider.Type.String())
	}

	// add image
	imageID := ""
	images, err := client.GetImages()
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to initialize Protos")
	}
	if id, found := images["protos-image"]; found == true {
		log.Infof("Found latest Protos image (%s) in your infra cloud account", id)
		imageID = id
	} else {
		// upload protos image
		log.Info("Latest Protos image not in your infra cloud account. Adding it.")
		imageID, err = client.AddImage("https://releases.protos.io/test/scaleway-efi.iso", "4b49901e65420b55170d95768f431595")
		if err != nil {
			return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to initialize Protos")
		}
	}

	// create SSH key used for instance
	log.Info("Generating SSH key for the new VM instance")
	key, err := ssh.GenerateKey()
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to initialize Protos")
	}

	// deploy a protos instance
	log.Infof("Deploying Protos instance '%s' using image '%s'", instanceName, imageID)
	vmID, err := client.NewInstance(instanceName, imageID, key.Public())
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to deploy Protos instance")
	}
	log.Infof("Instance with ID '%s' deployed", vmID)

	// get instance info
	instanceInfo, err := client.GetInstanceInfo(vmID)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to get Protos instance info")
	}
	// save of the instance information
	err = dbp.SaveInstance(instanceInfo)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to save instance '%s'", instanceName)
	}

	// create protos data volume
	log.Infof("Creating data volume for Protos instance '%s'", instanceName)
	volumeID, err := client.NewVolume(instanceName, 30000)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to create data volume")
	}

	// attach volume to instance
	err = client.AttachVolume(volumeID, vmID)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to attach volume to instance '%s'", instanceName)
	}

	// start protos instance
	log.Infof("Starting Protos instance '%s'", instanceName)
	err = client.StartInstance(vmID)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to start Protos instance")
	}

	// get instance info again
	instanceInfo, err = client.GetInstanceInfo(vmID)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrap(err, "Failed to get Protos instance info")
	}
	// final save of the instance information
	instanceInfo.KeySeed = key.Seed()
	err = dbp.SaveInstance(instanceInfo)
	if err != nil {
		return cloud.InstanceInfo{}, errors.Wrapf(err, "Failed to save instance '%s'", instanceName)
	}

	return instanceInfo, nil
}

func deleteInstance(name string) error {
	instance, err := dbp.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	cloudInfo, err := dbp.GetCloud(instance.CloudName)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloudInfo.Client()
	err = client.Init(cloudInfo.Auth, instance.Location)
	if err != nil {
		return errors.Wrapf(err, "Could not init cloud '%s'", name)
	}

	log.Infof("Stopping instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.StopInstance(instance.VMID)
	if err != nil {
		return errors.Wrapf(err, "Could not stop instance '%s'", name)
	}
	vmInfo, err := client.GetInstanceInfo(instance.VMID)
	if err != nil {
		return errors.Wrapf(err, "Failed to get details for instance '%s'", name)
	}
	log.Infof("Deleting instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.DeleteInstance(instance.VMID)
	if err != nil {
		return errors.Wrapf(err, "Could not delete instance '%s'", name)
	}
	for _, vol := range vmInfo.Volumes {
		log.Infof("Deleting volume '%s' (%s) for instance '%s'", vol.Name, vol.VolumeID, name)
		err = client.DeleteVolume(vol.VolumeID)
		if err != nil {
			log.Errorf("Failed to delete volume '%s': %s", vol.Name, err.Error())
		}
	}
	return dbp.DeleteInstance(name)
}

func startInstance(name string) error {
	instance, err := dbp.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	cloudInfo, err := dbp.GetCloud(instance.CloudName)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloudInfo.Client()
	err = client.Init(cloudInfo.Auth, instance.Location)
	if err != nil {
		return errors.Wrapf(err, "Could not init cloud '%s'", name)
	}

	log.Infof("Starting instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.StartInstance(instance.VMID)
	if err != nil {
		return errors.Wrapf(err, "Could not start instance '%s'", name)
	}
	return nil
}

func stopInstance(name string) error {
	instance, err := dbp.GetInstance(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve instance '%s'", name)
	}
	cloudInfo, err := dbp.GetCloud(instance.CloudName)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloudInfo.Client()
	err = client.Init(cloudInfo.Auth, instance.Location)
	if err != nil {
		return errors.Wrapf(err, "Could not init cloud '%s'", name)
	}

	log.Infof("Stopping instance '%s' (%s)", instance.Name, instance.VMID)
	err = client.StopInstance(instance.VMID)
	if err != nil {
		return errors.Wrapf(err, "Could not stop instance '%s'", name)
	}
	return nil
}

func tunnelInstance(name string) error {
	instanceInfo, err := dbp.GetInstance(name)
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
	instanceInfo, err := dbp.GetInstance(name)
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
