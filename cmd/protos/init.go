package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/db"
	ssh "github.com/protosio/cli/internal/ssh"
)

func protosInit() error {

	// create Protos DB
	dbPath, err := db.Init()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	dbp, err = db.Open(dbPath)
	if err != nil {
		return err
	}

	//
	// add cloud provider
	//

	// get a name to use internally for this specific cloud provider + credentials. This allows for adding multiple accounts of the same cloud
	cloudNameQuestion := []*survey.Question{{
		Name:     "name",
		Prompt:   &survey.Input{Message: "Write a name used to identify this cloud provider account internally:"},
		Validate: survey.Required,
	}}
	var cloudName string
	err = survey.Ask(cloudNameQuestion, &cloudName)

	client, err := addCloudProvider(cloudName)
	if err != nil {
		return err
	}

	//
	// Protos instance creation steps
	//

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

func surveySelect(options []string, message string) *survey.Select {
	return &survey.Select{
		Message: message,
		Options: options,
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
