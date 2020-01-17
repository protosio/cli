package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	ssh "github.com/protosio/cli/internal/ssh"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	log := logrus.New()
	var loglevel string
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
				Action: func(c *cli.Context) error {
					return protosInit(log)
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

func getUserDetailsQuestions(ud *userDetails) []*survey.Question {
	return []*survey.Question{
		{
			Name:      "username",
			Prompt:    &survey.Input{Message: "Username:"},
			Validate:  survey.Required,
			Transform: survey.ToLower,
		},
		{
			Name:      "name",
			Prompt:    &survey.Input{Message: "Name:"},
			Validate:  survey.Required,
			Transform: survey.Title,
		},
		{
			Name:     "password",
			Prompt:   &survey.Password{Message: "Password:"},
			Validate: survey.Required,
		},
		{
			Name:   "passwordconfirm",
			Prompt: &survey.Password{Message: "Confirm password:"},
			Validate: func(val interface{}) error {
				if str, ok := val.(string); ok && str != ud.Password {
					return fmt.Errorf("passwords don't match")
				}
				return nil
			},
		},
		{
			Name:     "domain",
			Prompt:   &survey.Input{Message: "Domain name (registered with one of the supported domain providers)"},
			Validate: survey.Required,
		},
	}
}

func getCloudProviderSelect(cloudProviders []string) *survey.Select {
	return &survey.Select{
		Message: "Choose one of the following supported cloud providers:",
		Options: cloudProviders,
	}
}

func getCloudProviderLocationSelect(providerName string, cloudProviderLocations []string) *survey.Select {
	return &survey.Select{
		Message: fmt.Sprintf("Choose one of the following supported locations supported for '%s':", providerName),
		Options: cloudProviderLocations,
	}
}

func getCloudCredentialsQuestions(providerName string, fields []string) []*survey.Question {
	qs := []*survey.Question{}
	for _, field := range fields {
		qs = append(qs, &survey.Question{
			Name:     field,
			Prompt:   &survey.Input{Message: providerName + " " + field + ":"},
			Validate: survey.Required})
	}
	return qs
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

func protosInit(log *logrus.Logger) error {

	// create config and state directory
	usr, _ := user.Current()
	protosDir := usr.HomeDir + "/.protos"
	log.Infof("Creating Protos directory '%s'", protosDir)
	err := os.MkdirAll(protosDir, os.FileMode(0600))
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}

	// select cloud provider
	var cloudProvider string
	cloudProviders := cloud.SupportedProviders()
	cloudProviderSelect := getCloudProviderSelect(cloudProviders)

	err = survey.AskOne(cloudProviderSelect, &cloudProvider)
	if err != nil {
		return err
	}

	client, err := cloud.NewClient(cloudProvider)
	if err != nil {
		return err
	}

	// get cloud provider credentials
	cloudCredentials := map[string]interface{}{}
	credFields := client.AuthFields()
	credentialsQuestions := getCloudCredentialsQuestions(cloudProvider, credFields)

	err = survey.Ask(credentialsQuestions, &cloudCredentials)
	if err != nil {
		return err
	}

	// get cloud provider location
	var cloudLocation string
	supportedLocations := client.SupportedLocations()
	cloudLocationQuestions := getCloudProviderLocationSelect(cloudProvider, supportedLocations)

	err = survey.AskOne(cloudLocationQuestions, &cloudLocation)
	if err != nil {
		return err
	}

	// init cloud client
	err = client.Init(transformCredentials(cloudCredentials), cloudLocation)
	if err != nil {
		return err
	}

	imageID := ""
	images, err := client.GetImages()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	if id, found := images["protos-image"]; found == true {
		log.Infof("Found latest Protos image (%s) in your infra cloud account", id)
		imageID = id
	} else {
		// upload protos image
		log.Info("Latest Protos image not in your infra cloud account. Adding it.")
		imageID, err = client.AddImage("https://releases.protos.io/test/scaleway-efi.iso", "4b49901e65420b55170d95768f431595")
		if err != nil {
			return errors.Wrap(err, "Failed to initialize Protos")
		}
	}

	// create SSH key used for instance
	log.Info("Generating SSH key for the new VM instance")
	key, err := ssh.GenerateKey()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}

	// deploy a protos instance
	vmName := "protos1"
	log.Infof("Deploying Protos instance '%s' using image '%s'", vmName, imageID)
	vmID, err := client.NewInstance(vmName, imageID, key.Public())
	if err != nil {
		return errors.Wrap(err, "Failed to deploy Protos instance")
	}
	log.Infof("Instance with ID '%s' deployed", vmID)

	// create protos data volume
	log.Infof("Creating data volume for Protos instance '%s'", vmName)
	volumeID, err := client.NewVolume(vmName, 30000)
	if err != nil {
		return errors.Wrap(err, "Failed to create data volume")
	}

	// attach volume to instance
	err = client.AttachVolume(volumeID, vmID)
	if err != nil {
		return errors.Wrapf(err, "Failed to attach volume to instance '%s'", vmName)
	}

	// start protos instance
	log.Infof("Starting Protos instance '%s'", vmName)
	err = client.StartInstance(vmID)
	if err != nil {
		return errors.Wrap(err, "Failed to start Protos instance")
	}

	// get info about the instance
	instanceInfo, err := client.GetInstanceInfo(vmID)
	if err != nil {
		return errors.Wrap(err, "Failed to get Protos instance info")
	}

	// test SSH and create SSH tunnel used for initialisation
	tempClient, err := ssh.NewConnection(instanceInfo.PublicIP, "root", key.SSHAuth(), 10)
	if err != nil {
		return errors.Wrap(err, "Failed to connect to Protos instance via SSH")
	}
	tempClient.Close()
	log.Info("Instance is ready and accepting SSH connections")

	log.Infof("Creating SSH tunnel to the new VM, using ip '%s'", instanceInfo.PublicIP)
	tunnel := ssh.NewTunnel(instanceInfo.PublicIP+":22", "root", key.SSHAuth(), "localhost:8080", log)
	localPort, err := tunnel.Start()
	if err != nil {
		return errors.Wrap(err, "Error while creating the SSH tunnel")
	}

	quit := make(chan interface{}, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go catchSignals(sigs, quit)

	log.Infof("SSH tunnel ready. Please do the setup using a browser at 'http://localhost:%d/'. Once finished, press CTRL+C to continue", localPort)

	// waiting for a SIGTERM or SIGINT
	<-quit

	log.Info("CTRL+C received. Shutting down the SSH tunnel")
	err = tunnel.Close()
	if err != nil {
		return errors.Wrap(err, "Error while terminating the SSH tunnel")
	}
	log.Info("SSH tunnel terminated successfully")
	log.Infof("Protos instance '%s' - '%s' deployed successfully", vmName, instanceInfo.PublicIP)

	// // get user details
	// userDetails := userDetails{}
	// userDetailsQuestions := getUserDetailsQuestions(&userDetails)
	// err = survey.Ask(userDetailsQuestions, &userDetails)
	// if err != nil {
	// 	return err
	// }

	return nil
}
